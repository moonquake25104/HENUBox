package com.xique.assistant.henu.classroom;

import com.xique.assistant.henu.crypto.SessionCrypto;
import com.xique.assistant.henu.ops.HenuError;
import com.xique.assistant.henu.ops.HenuService;
import com.xique.assistant.henu.storage.ConnectionRow;
import com.xique.assistant.henu.storage.LocalDb;
import com.xique.assistant.henu.storage.LocalSettingsStore;

import org.json.JSONArray;
import org.json.JSONObject;

import java.nio.charset.StandardCharsets;
import java.time.LocalDate;
import java.time.temporal.ChronoUnit;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashMap;
import java.util.HashSet;
import java.util.Iterator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Set;

/**
 * Read-only classroom availability orchestration for the mobile bridge.
 * Short in-process cache only: the catalog bundle lives 24 h, one building
 * week matrix 5 min, at most 8 building-weeks. Nothing here writes SQLite,
 * touches local_courses / local_source_sessions / local_semesters or bumps a
 * workspace revision, and no school session state is persisted from here.
 *
 * Integrity rules (task book 2026-10-29): unknown is never free; results are
 * only published after the connection generation and subject fingerprint are
 * re-verified; cached entries are bound to the generation, term and query
 * scope they were fetched for; a failed refresh serves the same-scope cache
 * as STALE with a safe reason, never another building's data.
 */
public final class ClassroomService {

    private static final long CATALOG_TTL_MS = 24 * 60 * 60 * 1000L;
    private static final long WEEK_TTL_MS = 5 * 60 * 1000L;
    private static final int MAX_WEEK_CACHE = 8;
    private static final int MAX_ROOMS_CACHE = 16;
    private static final int PERIOD_COUNT = 13;

    /** Parser issue codes that describe structure and are worth surfacing. */
    private static final Set<String> STRUCTURAL_CODES = new HashSet<>(Arrays.asList(
            "DUPLICATE_PERIOD_ROW", "UNEXPECTED_MERGE",
            "UNEXPECTED_ROW_WIDTH", "MISSING_PERIOD", "INVALID_SEATS", "INVALID_WEEK"));

    private final LocalDb db;
    private final Object lock = new Object();
    private final ClassroomClient.Transport transport;
    private final MatrixParser parser;

    /**
     * Parser seam: production binds the Go AAR via {@link ClassroomParse};
     * instrumented tests on emulators inject a pure-Java stand-in because the
     * arm64 Go runtime SIGILLs under ndk_translation (berberis).
     */
    @FunctionalInterface
    public interface MatrixParser {
        JSONObject parse(byte[] raw) throws HenuError;
    }

    private static final class Campus {
        final String id;
        final String name;
        final List<ClassroomClient.Entry> buildings = new ArrayList<>();

        Campus(String id, String name) {
            this.id = id;
            this.name = name;
        }
    }

    private static final class CatalogRoom {
        final String id;
        final String baseName;
        final String type;
        final int seats;

        CatalogRoom(String id, String baseName, String type, int seats) {
            this.id = id;
            this.baseName = baseName;
            this.type = type;
            this.seats = seats;
        }
    }

    private static final class Catalog {
        final String workspace;
        final long generation;
        final String year;
        final String termCode;
        final String label;
        final long fetchedAt;
        final List<Campus> campuses = new ArrayList<>();
        final Map<String, List<CatalogRoom>> rooms = new LinkedHashMap<>();

        Catalog(String workspace, long generation, String year, String termCode, String label, long fetchedAt) {
            this.workspace = workspace;
            this.generation = generation;
            this.year = year;
            this.termCode = termCode;
            this.label = label;
            this.fetchedAt = fetchedAt;
        }

        boolean matches(String ws, long gen) {
            return workspace.equals(ws) && generation == gen;
        }
    }

    /** One building week: all 7 days × 13 periods, projected per date at serve time. */
    private static final class WeekEntry {
        final long generation;
        final String year;
        final String termCode;
        final String campusId;
        final String campusName;
        final String buildingId;
        final String buildingName;
        final int week;
        final long fetchedAt;
        final JSONArray rooms;
        final JSONArray issues;

        WeekEntry(long generation, String year, String termCode,
                  String campusId, String campusName, String buildingId, String buildingName,
                  int week, long fetchedAt, JSONArray rooms, JSONArray issues) {
            this.generation = generation;
            this.year = year;
            this.termCode = termCode;
            this.campusId = campusId;
            this.campusName = campusName;
            this.buildingId = buildingId;
            this.buildingName = buildingName;
            this.week = week;
            this.fetchedAt = fetchedAt;
            this.rooms = rooms;
            this.issues = issues;
        }
    }

    private Catalog catalog;
    private final LinkedHashMap<String, WeekEntry> weekCache = new LinkedHashMap<>(16, 0.75f, true);
    private final Map<String, Pending> pending = new HashMap<>();

    private String seenWorkspace;
    private long seenGeneration = -1;
    private String seenFingerprint;

    /** Shared in-flight slot so concurrent same-scope queries cost one request. */
    private static final class Pending {
        boolean done;
        boolean failed;
        HenuError error;
        Object result;
    }

    public ClassroomService(LocalDb db) {
        this(db, null, null);
    }

    public ClassroomService(LocalDb db, ClassroomClient.Transport transport) {
        this(db, transport, null);
    }

    public ClassroomService(LocalDb db, ClassroomClient.Transport transport, MatrixParser parser) {
        this.db = db;
        this.transport = transport;
        this.parser = parser != null ? parser : ClassroomParse::parseWeekMatrix;
    }

    // ------------------------------------------------------------------
    // Public API
    // ------------------------------------------------------------------

    public JSONObject getBuildings(String workspace) throws HenuError {
        requireWorkspace(workspace);
        ConnectionRow row = requireConnection(workspace);
        watchGeneration(workspace, row);
        String key = "catalog:" + workspace + ":" + row.generation;
        synchronized (lock) {
            if (catalog != null && catalog.matches(workspace, row.generation)
                    && System.currentTimeMillis() - catalog.fetchedAt <= CATALOG_TTL_MS) {
                recheckUnchanged(workspace, row);
                return buildingsPayload(row.generation, catalog, "FRESH", null);
            }
            if (pending.containsKey(key)) return awaitCatalog(key, workspace, row);
            pending.put(key, new Pending());
        }
        try {
            Catalog fresh = fetchCatalog(workspace, row);
            synchronized (lock) {
                recheckUnchanged(workspace, row);
                catalog = fresh;
                Pending p = pending.remove(key);
                if (p != null) {
                    p.result = fresh;
                    p.done = true;
                    lock.notifyAll();
                }
                return buildingsPayload(row.generation, fresh, "LIVE", null);
            }
        } catch (HenuError e) {
            synchronized (lock) {
                Pending p = pending.remove(key);
                if (p != null) {
                    p.failed = true;
                    p.error = e;
                    p.done = true;
                    lock.notifyAll();
                }
                if (catalog != null && catalog.matches(workspace, row.generation)
                        && upstreamFailure(e.code)) {
                    recheckUnchanged(workspace, row);
                    return buildingsPayload(row.generation, catalog, "STALE", e.code);
                }
            }
            throw withoutCache(e);
        }
    }

    public JSONObject getDay(String workspace, String semester, String date,
                             String campusId, String buildingId, boolean refresh) throws HenuError {
        requireWorkspace(workspace);
        String sem = requireSemester(semester);
        requireDate(date);
        ClassroomClient.requireId(campusId);
        ClassroomClient.requireId(buildingId);
        ConnectionRow row = requireConnection(workspace);
        watchGeneration(workspace, row);

        JSONObject settings = LocalSettingsStore.readStored(db, workspace, sem);
        if (!sem.equals(semesterFor(row))) {
            throw new HenuError("UNSUPPORTED_TERM", "本机学期与当前学校学期不一致");
        }
        int[] weekWeekday = weekAndWeekday(settings, date);
        int week = weekWeekday[0];
        int weekday = weekWeekday[1];

        // The published term is verified equal to the connection term before
        // any matrix is fetched, so the connection term identifies the scope.
        String key = weekKey(workspace, row.generation, row.termYear, row.termTerm,
                campusId, buildingId, week);
        synchronized (lock) {
            WeekEntry entry = weekCache.get(key);
            if (entry != null && !refresh
                    && System.currentTimeMillis() - entry.fetchedAt <= WEEK_TTL_MS) {
                recheckUnchanged(workspace, row);
                return dayPayload(entry, sem, date, week, weekday, "FRESH", null);
            }
            if (pending.containsKey(key)) return awaitWeek(key, workspace, row, sem, date, week, weekday);
            pending.put(key, new Pending());
        }
        try {
            ClassroomClient client = new ClassroomClient(unseal(workspace, row), transport);
            Catalog cat = ensureCatalog(workspace, row, campusId, buildingId, client);
            WeekEntry fresh = fetchWeek(workspace, row, cat, campusId, buildingId, week, client);
            synchronized (lock) {
                recheckUnchanged(workspace, row);
                weekCache.put(weekKey(workspace, row.generation, fresh.year, fresh.termCode,
                        campusId, buildingId, week), fresh);
                trimWeekCache();
                Pending p = pending.remove(key);
                if (p != null) {
                    p.result = fresh;
                    p.done = true;
                    lock.notifyAll();
                }
                return dayPayload(fresh, sem, date, week, weekday, "LIVE", null);
            }
        } catch (HenuError e) {
            synchronized (lock) {
                Pending p = pending.remove(key);
                if (p != null) {
                    p.failed = true;
                    p.error = e;
                    p.done = true;
                    lock.notifyAll();
                }
                WeekEntry entry = weekCache.get(key);
                if (entry != null && upstreamFailure(e.code)) {
                    recheckUnchanged(workspace, row);
                    return dayPayload(entry, sem, date, week, weekday, "STALE", e.code);
                }
            }
            throw withoutCache(e);
        }
    }

    // ------------------------------------------------------------------
    // Network steps
    // ------------------------------------------------------------------

    private Catalog fetchCatalog(String workspace, ConnectionRow row) throws HenuError {
        ClassroomClient client = new ClassroomClient(unseal(workspace, row), transport);
        String[] term = resolvePublishedTerm(client.publishedTerms(), row);
        Catalog out = new Catalog(workspace, row.generation, term[0], term[1],
                row.termLabel == null ? "" : row.termLabel, System.currentTimeMillis());
        for (ClassroomClient.Entry campus : client.campuses()) {
            Campus c = new Campus(campus.code, campus.name);
            for (ClassroomClient.Entry building : client.buildingsOfCampus(campus.code)) {
                c.buildings.add(building);
            }
            out.campuses.add(c);
        }
        return out;
    }

    /** Catalog plus the room roster of the target building, for one generation. */
    private Catalog ensureCatalog(String workspace, ConnectionRow row,
                                  String campusId, String buildingId, ClassroomClient client) throws HenuError {
        boolean haveFreshCatalog;
        List<CatalogRoom> cachedRooms = null;
        synchronized (lock) {
            haveFreshCatalog = catalog != null && catalog.matches(workspace, row.generation)
                    && System.currentTimeMillis() - catalog.fetchedAt <= CATALOG_TTL_MS;
            if (haveFreshCatalog) {
                requireCatalogBuilding(catalog, campusId, buildingId);
                cachedRooms = catalog.rooms.get(roomKey(campusId, buildingId));
            }
        }
        Catalog fresh = null;
        List<CatalogRoom> myRooms = cachedRooms;
        if (haveFreshCatalog) {
            // Reuse the cached campuses/buildings/room roster; fetch only what
            // is missing. refresh=true must not force the catalog open again.
            if (myRooms == null) myRooms = fetchRooms(client, campusId, buildingId);
        } else {
            String[] term = resolvePublishedTerm(client.publishedTerms(), row);
            fresh = new Catalog(workspace, row.generation, term[0], term[1],
                    row.termLabel == null ? "" : row.termLabel, System.currentTimeMillis());
            boolean campusFound = false;
            boolean buildingFound = false;
            for (ClassroomClient.Entry campus : client.campuses()) {
                Campus c = new Campus(campus.code, campus.name);
                boolean here = campus.code.equals(campusId);
                for (ClassroomClient.Entry building : client.buildingsOfCampus(campus.code)) {
                    c.buildings.add(building);
                    if (here && building.code.equals(buildingId)) buildingFound = true;
                }
                if (here) campusFound = true;
                fresh.campuses.add(c);
            }
            if (!campusFound || !buildingFound) throw HenuError.notFound("目录中没有该校区或教学楼");
            myRooms = fetchRooms(client, campusId, buildingId);
            fresh.rooms.put(roomKey(campusId, buildingId), myRooms);
        }
        synchronized (lock) {
            recheckUnchanged(workspace, row);
            Catalog current = catalog != null && catalog.matches(workspace, row.generation)
                    && System.currentTimeMillis() - catalog.fetchedAt <= CATALOG_TTL_MS
                    ? catalog : fresh;
            if (current != null && !current.rooms.containsKey(roomKey(campusId, buildingId)) && myRooms != null) {
                current.rooms.put(roomKey(campusId, buildingId), myRooms);
                trimRoomsCache();
            }
            if (current != null) catalog = current;
            return current;
        }
    }

    private List<CatalogRoom> fetchRooms(ClassroomClient client, String campusId, String buildingId)
            throws HenuError {
        List<CatalogRoom> rooms = new ArrayList<>();
        for (ClassroomClient.Entry room : client.roomsOfBuilding(campusId, buildingId)) {
            rooms.add(parseCatalogRoom(room));
        }
        return rooms;
    }

    /** Parse the directory's trailing [容量][类型] suffixes without inferring floor. */
    private static CatalogRoom parseCatalogRoom(ClassroomClient.Entry entry) {
        String name = entry.name == null ? "" : entry.name.trim();
        String type = null;
        int seats = 0;
        int end = name.length();
        while (end > 0 && name.charAt(end - 1) == ']') {
            int open = name.lastIndexOf('[', end - 2);
            if (open < 0) break;
            String token = name.substring(open + 1, end - 1).trim();
            if (token.matches("[0-9]{1,5}") && seats == 0) {
                seats = Integer.parseInt(token);
            } else if (type == null && !token.isEmpty()) {
                type = token;
            } else {
                break;
            }
            end = open;
        }
        String base = name.substring(0, end).trim();
        return new CatalogRoom(entry.code, base, type, seats);
    }

    private WeekEntry fetchWeek(String workspace, ConnectionRow row, Catalog cat,
                                String campusId, String buildingId, int week, ClassroomClient client)
            throws HenuError {
        byte[] raw = client.fetchWeekMatrix(campusId, buildingId, cat.year, cat.termCode, week);
        return assembleWeek(row.generation, cat, campusId, buildingId, week, parser.parse(raw));
    }

    // ------------------------------------------------------------------
    // Matrix assembly: catalog names must uniquely match before any room id
    // is assigned; missing or ambiguous rooms stay UNKNOWN, never FREE.
    // ------------------------------------------------------------------

    private WeekEntry assembleWeek(long generation, Catalog cat, String campusId,
                                   String buildingId, int week, JSONObject parsed) throws HenuError {
        Campus campus = null;
        String buildingName = null;
        for (Campus c : cat.campuses) {
            if (c.id.equals(campusId)) {
                campus = c;
                for (ClassroomClient.Entry b : c.buildings) {
                    if (b.code.equals(buildingId)) buildingName = b.name;
                }
            }
        }
        if (campus == null || buildingName == null) throw HenuError.notFound("目录中没有该校区或教学楼");

        List<CatalogRoom> roster = cat.rooms.get(roomKey(campusId, buildingId));
        if (roster == null) roster = new ArrayList<>();
        Map<String, CatalogRoom> rosterById = new LinkedHashMap<>();
        for (CatalogRoom r : roster) rosterById.put(r.id, r);

        JSONArray matrixRooms = parsed.optJSONArray("rooms");
        if (matrixRooms == null) matrixRooms = new JSONArray();
        int anonymous = parsed.optInt("anonymous_tables", 0);
        JSONArray parserIssues = parsed.optJSONArray("issues");
        if (parserIssues == null) parserIssues = new JSONArray();

        Map<String, List<JSONObject>> byName = new LinkedHashMap<>();
        for (int i = 0; i < matrixRooms.length(); i++) {
            JSONObject room = matrixRooms.optJSONObject(i);
            if (room == null) continue;
            if (room.optInt("week", -1) != week) {
                throw new HenuError("UNRECOGNIZED_DOCUMENT", "周次与请求不符");
            }
            String name = room.optString("name", "");
            List<JSONObject> list = byName.get(name);
            if (list == null) {
                list = new ArrayList<>();
                byName.put(name, list);
            }
            list.add(room);
        }

        JSONArray issues = new JSONArray();
        if (anonymous > 0) addIssue(issues, "ANONYMOUS_TABLE", null);
        for (int i = 0; i < parserIssues.length(); i++) {
            JSONObject issue = parserIssues.optJSONObject(i);
            if (issue == null) continue;
            String code = issue.optString("code", "");
            if (!STRUCTURAL_CODES.contains(code)) continue;
            String roomName = issue.optString("room", "");
            String roomId = null;
            for (CatalogRoom r : roster) {
                if (r.baseName.equals(roomName)) {
                    roomId = r.id;
                    break;
                }
            }
            addIssue(issues, code, roomId);
        }

        Map<String, Integer> catalogNameCounts = new HashMap<>();
        Set<String> catalogIds = new HashSet<>();
        for (CatalogRoom r : roster) {
            if (!catalogIds.add(r.id)) throw new HenuError("UNRECOGNIZED_DOCUMENT", "教室目录存在重复编码");
            catalogNameCounts.put(r.baseName, catalogNameCounts.getOrDefault(r.baseName, 0) + 1);
        }
        Set<String> matched = new HashSet<>();
        JSONArray roomsOut = new JSONArray();
        for (CatalogRoom catalogRoom : roster) {
            boolean duplicateBase = catalogNameCounts.get(catalogRoom.baseName) > 1;
            List<JSONObject> matches = byName.get(catalogRoom.baseName);
            JSONObject matrix = null;
            if (duplicateBase) {
                addIssue(issues, "DUPLICATE_CATALOG_ROOM", catalogRoom.id);
            } else if (matches == null) {
                addIssue(issues, "MATRIX_ROOM_MISSING", catalogRoom.id);
            } else if (matches.size() > 1) {
                addIssue(issues, "DUPLICATE_ROOM", catalogRoom.id);
            } else {
                matrix = matches.get(0);
                matched.add(catalogRoom.baseName);
            }
            roomsOut.put(roomWithDays(catalogRoom, matrix));
        }
        for (String name : byName.keySet()) {
            if (!matched.contains(name)) addIssue(issues, "MATRIX_ROOM_UNLISTED", null);
        }

        return new WeekEntry(generation, cat.year, cat.termCode, campusId, campus.name,
                buildingId, buildingName, week, System.currentTimeMillis(), roomsOut, issues);
    }

    private static JSONObject roomWithDays(CatalogRoom catalogRoom, JSONObject matrix) throws HenuError {
        try {
            JSONObject out = new JSONObject();
            out.put("id", catalogRoom.id);
            out.put("name", catalogRoom.baseName);
            String type = matrix == null ? "" : matrix.optString("type", "");
            if (type.isEmpty() && catalogRoom.type != null) type = catalogRoom.type;
            out.put("type", type.isEmpty() ? JSONObject.NULL : type);
            int seats = matrix == null ? 0 : matrix.optInt("seats", 0);
            if (seats <= 0) seats = catalogRoom.seats;
            out.put("seats", seats > 0 ? seats : JSONObject.NULL);
            out.put("floor", JSONObject.NULL);
            JSONArray days = new JSONArray();
            for (int day = 0; day < 7; day++) {
                JSONArray row = new JSONArray();
                for (int period = 1; period <= PERIOD_COUNT; period++) {
                    String status = "UNKNOWN";
                    if (matrix != null) {
                        JSONObject periods = matrix.optJSONObject("periods");
                        JSONArray states = periods == null ? null : periods.optJSONArray(String.valueOf(period));
                        if (states != null && states.length() == 7) {
                            status = states.optString(day, "UNKNOWN");
                            if (!"FREE".equals(status) && !"OCCUPIED".equals(status)) status = "UNKNOWN";
                        }
                    }
                    row.put(status);
                }
                days.put(row);
            }
            out.put("days", days);
            return out;
        } catch (org.json.JSONException e) {
            throw HenuError.storage("教室结果序列化失败");
        }
    }

    private static void addIssue(JSONArray issues, String code, String roomId) throws HenuError {
        try {
            issues.put(new JSONObject()
                    .put("code", code)
                    .put("room_id", roomId == null ? JSONObject.NULL : roomId));
        } catch (org.json.JSONException e) {
            throw HenuError.storage("教室结果序列化失败");
        }
    }

    // ------------------------------------------------------------------
    // Payloads
    // ------------------------------------------------------------------

    private JSONObject buildingsPayload(long generation, Catalog cat, String state, String reason)
            throws HenuError {
        try {
            JSONObject out = new JSONObject();
            out.put("connection_generation", generation);
            out.put("term", new JSONObject()
                    .put("year", cat.year)
                    .put("term_code", cat.termCode)
                    .put("label", cat.label));
            out.put("fetched_at", HenuService.iso(cat.fetchedAt));
            out.put("cache", cacheInfo(state, reason));
            JSONArray campuses = new JSONArray();
            for (Campus c : cat.campuses) {
                JSONArray buildings = new JSONArray();
                for (ClassroomClient.Entry b : c.buildings) {
                    buildings.put(new JSONObject().put("id", b.code).put("name", b.name));
                }
                campuses.put(new JSONObject()
                        .put("id", c.id)
                        .put("name", c.name)
                        .put("buildings", buildings));
            }
            out.put("campuses", campuses);
            return out;
        } catch (org.json.JSONException e) {
            throw HenuError.storage("教室目录序列化失败");
        }
    }

    private JSONObject dayPayload(WeekEntry entry, String sem, String date, int week,
                                  int weekday, String state, String reason) throws HenuError {
        try {
            JSONObject out = new JSONObject();
            out.put("connection_generation", entry.generation);
            out.put("semester", sem);
            out.put("date", date);
            out.put("week", week);
            out.put("weekday", weekday);
            out.put("campus", new JSONObject()
                    .put("id", entry.campusId)
                    .put("name", entry.campusName));
            out.put("building", new JSONObject()
                    .put("id", entry.buildingId)
                    .put("name", entry.buildingName));
            out.put("period_count", PERIOD_COUNT);
            out.put("source", "HENU_PUBLIC_QUERY");
            out.put("policy", new JSONObject().put("retained_as_occupied", true));
            out.put("fetched_at", HenuService.iso(entry.fetchedAt));
            out.put("cache", cacheInfo(state, reason));
            JSONArray roomsOut = new JSONArray();
            int unknownCells = 0;
            int completeRooms = 0;
            for (int i = 0; i < entry.rooms.length(); i++) {
                JSONObject room = entry.rooms.optJSONObject(i);
                if (room == null) continue;
                JSONArray days = room.optJSONArray("days");
                JSONArray row = days == null || weekday - 1 >= days.length()
                        ? null : days.optJSONArray(weekday - 1);
                JSONArray cells = new JSONArray();
                int roomUnknown = 0;
                for (int period = 1; period <= PERIOD_COUNT; period++) {
                    String status = row != null && period - 1 < row.length()
                            ? row.optString(period - 1, "UNKNOWN") : "UNKNOWN";
                    if ("UNKNOWN".equals(status)) roomUnknown++;
                    cells.put(new JSONObject().put("period", period).put("status", status));
                }
                unknownCells += roomUnknown;
                if (roomUnknown == 0) completeRooms++;
                roomsOut.put(new JSONObject()
                        .put("id", room.optString("id"))
                        .put("name", room.optString("name"))
                        .put("type", room.opt("type"))
                        .put("seats", room.opt("seats"))
                        .put("floor", JSONObject.NULL)
                        .put("cells", cells));
            }
            out.put("coverage", new JSONObject()
                    .put("roster_rooms", roomsOut.length())
                    .put("complete_rooms", completeRooms)
                    .put("unknown_cells", unknownCells));
            out.put("issues", entry.issues);
            out.put("rooms", roomsOut);
            return out;
        } catch (org.json.JSONException e) {
            throw HenuError.storage("教室结果序列化失败");
        }
    }

    private static JSONObject cacheInfo(String state, String reason) throws org.json.JSONException {
        return new JSONObject()
                .put("state", state)
                .put("reason", reason == null ? JSONObject.NULL : reason);
    }

    // ------------------------------------------------------------------
    // In-flight merge waiters (called while holding lock)
    // ------------------------------------------------------------------

    private JSONObject awaitCatalog(String key, String workspace, ConnectionRow row) throws HenuError {
        Pending p = pending.get(key);
        while (p != null && !p.done) {
            try {
                lock.wait();
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw HenuError.storage("查询被中断");
            }
        }
        if (p != null && p.done && !p.failed && p.result instanceof Catalog) {
            recheckUnchanged(workspace, row);
            Catalog c = (Catalog) p.result;
            if (c.matches(workspace, row.generation)) {
                boolean fresh = System.currentTimeMillis() - c.fetchedAt <= CATALOG_TTL_MS;
                return buildingsPayload(row.generation, c, fresh ? "LIVE" : "FRESH", null);
            }
        }
        if (p != null && p.failed && p.error != null) {
            if (catalog != null && catalog.matches(workspace, row.generation)
                    && upstreamFailure(p.error.code)) {
                recheckUnchanged(workspace, row);
                return buildingsPayload(row.generation, catalog, "STALE", p.error.code);
            }
            throw withoutCache(p.error);
        }
        throw HenuError.storage("查询状态异常");
    }

    private JSONObject awaitWeek(String key, String workspace, ConnectionRow connection,
                                String sem, String date, int week, int weekday)
            throws HenuError {
        Pending p = pending.get(key);
        while (p != null && !p.done) {
            try {
                lock.wait();
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw HenuError.storage("查询被中断");
            }
        }
        if (p != null && p.done && !p.failed && p.result instanceof WeekEntry) {
            recheckUnchanged(workspace, connection);
            return dayPayload((WeekEntry) p.result, sem, date, week, weekday, "LIVE", null);
        }
        if (p != null && p.failed && p.error != null) {
            WeekEntry entry = weekCache.get(key);
            if (entry != null && upstreamFailure(p.error.code)) {
                recheckUnchanged(workspace, connection);
                return dayPayload(entry, sem, date, week, weekday, "STALE", p.error.code);
            }
            throw withoutCache(p.error);
        }
        throw HenuError.storage("查询状态异常");
    }

    /** Only upstream failures may be served from an older same-scope cache. */
    private static boolean upstreamFailure(String code) {
        return "UPSTREAM_AUTH_REQUIRED".equals(code)
                || "UPSTREAM_UNAVAILABLE".equals(code)
                || "UPSTREAM_TLS_ERROR".equals(code)
                || "UNRECOGNIZED_DOCUMENT".equals(code);
    }

    /** First attempt with no cache: network-level unavailability becomes OFFLINE_NO_CACHE. */
    private static HenuError withoutCache(HenuError e) {
        if ("UPSTREAM_UNAVAILABLE".equals(e.code)) {
            return new HenuError("OFFLINE_NO_CACHE", "当前无法连接学校，且没有上次查询缓存");
        }
        return e;
    }

    // ------------------------------------------------------------------
    // Validation and session helpers
    // ------------------------------------------------------------------

    private void requireWorkspace(String workspace) throws HenuError {
        if (workspace == null || !workspace.equals(db.ensureWorkspace())) {
            throw HenuError.notFound("工作区不存在");
        }
    }

    private ConnectionRow requireConnection(String workspace) throws HenuError {
        ConnectionRow row = ConnectionRow.read(db, workspace);
        if (row == null || row.ciphertext == null) {
            throw new HenuError("NOT_CONFIGURED", "尚未连接河大");
        }
        if ("AUTH_REQUIRED".equals(row.state)) {
            throw new HenuError("UPSTREAM_AUTH_REQUIRED", "学校会话已失效，请重新授权");
        }
        return row;
    }

    private Map<String, String> unseal(String workspace, ConnectionRow row) throws HenuError {
        try {
            JSONObject payload = new JSONObject(new String(
                    SessionCrypto.open(row.ciphertext, SessionCrypto.aad(workspace, "henu", row.generation)),
                    StandardCharsets.UTF_8));
            JSONObject jar = payload.getJSONObject("cookies");
            Map<String, String> cookies = new LinkedHashMap<>();
            Iterator<String> it = jar.keys();
            while (it.hasNext()) {
                String name = it.next();
                cookies.put(name, jar.getString(name));
            }
            return cookies;
        } catch (Exception e) {
            throw new HenuError("STORAGE_UNAVAILABLE", "学校会话无法读取");
        }
    }

    /** Clears every cached classroom artifact when the school connection changed. */
    private void watchGeneration(String workspace, ConnectionRow row) throws HenuError {
        synchronized (lock) {
            boolean changed = !workspace.equals(seenWorkspace)
                    || row.generation != seenGeneration
                    || !Objects.equals(row.subjectFingerprint, seenFingerprint);
            if (changed) {
                catalog = null;
                weekCache.clear();
                for (Pending p : pending.values()) {
                    p.failed = true;
                    p.error = new HenuError("CONNECTION_CHANGED", "学校连接已变化，结果作废");
                    p.done = true;
                }
                pending.clear();
                lock.notifyAll();
                seenWorkspace = workspace;
                seenGeneration = row.generation;
                seenFingerprint = row.subjectFingerprint;
            }
        }
    }

    private void recheckUnchanged(String workspace, ConnectionRow row) throws HenuError {
        ConnectionRow now = ConnectionRow.read(db, workspace);
        if (now == null || now.ciphertext == null || now.generation != row.generation
                || !Objects.equals(row.subjectFingerprint, now.subjectFingerprint)) {
            throw new HenuError("CONNECTION_CHANGED", "学校连接已变化，结果作废");
        }
    }

    private String[] resolvePublishedTerm(List<ClassroomClient.Entry> terms, ConnectionRow row)
            throws HenuError {
        if (row.termYear == null || row.termTerm == null
                || !row.termYear.matches("[0-9]{4}") || !row.termTerm.matches("[0-9]{1,2}")) {
            throw new HenuError("UNSUPPORTED_TERM", "本机连接学期信息无效");
        }
        String want = row.termYear + "," + row.termTerm;
        for (ClassroomClient.Entry e : terms) {
            if (want.equals(e.code)) return new String[]{row.termYear, row.termTerm};
        }
        throw new HenuError("UNSUPPORTED_TERM", "学校发布学期与本机连接学期不一致");
    }

    private String requireSemester(String semester) throws HenuError {
        if (semester == null || !semester.matches("[A-Za-z0-9_-]{1,64}")) {
            throw HenuError.invalid("semester 不合法");
        }
        return semester;
    }

    private void requireDate(String date) throws HenuError {
        if (date == null || !date.matches("[0-9]{4}-[0-9]{2}-[0-9]{2}")) {
            throw HenuError.invalid("日期格式无效");
        }
        try {
            LocalDate.parse(date);
        } catch (Exception e) {
            throw HenuError.invalid("日期不存在");
        }
    }

    private int[] weekAndWeekday(JSONObject settings, String date) throws HenuError {
        if (settings == null) throw new HenuError("NOT_CONFIGURED", "未配置该学期作息");
        String label = settings.optString("semester_label", "");
        JSONObject anchor = settings.optJSONObject("week_anchor");
        String start = anchor == null ? "" : anchor.optString("start_date", "");
        boolean confirmed = anchor != null && anchor.optBoolean("confirmed", false);
        int totalWeeks = settings.optInt("total_weeks", 0);
        if (label.isEmpty() || !confirmed || start.isEmpty() || totalWeeks < 1) {
            throw new HenuError("NOT_CONFIGURED", "未配置该学期作息");
        }
        LocalDate anchorDate;
        LocalDate day;
        try {
            anchorDate = LocalDate.parse(start);
            day = LocalDate.parse(date);
        } catch (Exception e) {
            throw new HenuError("NOT_CONFIGURED", "学期锚点无效");
        }
        int week = (int) Math.floorDiv(ChronoUnit.DAYS.between(anchorDate, day), 7) + 1;
        int weekday = day.getDayOfWeek().getValue();
        if (week < 1 || week > totalWeeks) {
            throw new HenuError("DATE_OUTSIDE_TERM", "日期超出已配置学期");
        }
        return new int[]{week, weekday};
    }

    /** Catalog display names carry [容量][类型] suffixes; the base name is the match key. */
    private static String baseRoomName(String name) {
        String result = name == null ? "" : name.trim();
        while (result.endsWith("]")) {
            int open = result.lastIndexOf('[');
            if (open < 0) break;
            result = result.substring(0, open).trim();
        }
        return result;
    }

    private void trimWeekCache() {
        Iterator<String> it = weekCache.keySet().iterator();
        while (weekCache.size() > MAX_WEEK_CACHE && it.hasNext()) {
            it.next();
            it.remove();
        }
    }

    private void trimRoomsCache() {
        Iterator<String> it = catalog.rooms.keySet().iterator();
        while (catalog.rooms.size() > MAX_ROOMS_CACHE && it.hasNext()) {
            it.next();
            it.remove();
        }
    }

    private static String weekKey(String workspace, long generation, String year, String termCode,
                                  String campusId, String buildingId, int week) {
        return workspace + "|" + generation + "|" + year + "|" + termCode + "|" + campusId
                + "|" + buildingId + "|" + week + "|retained=1";
    }

    private static String roomKey(String campusId, String buildingId) {
        return campusId + ":" + buildingId;
    }

    private static void requireCatalogBuilding(Catalog cat, String campusId, String buildingId) throws HenuError {
        for (Campus c : cat.campuses) {
            if (!c.id.equals(campusId)) continue;
            for (ClassroomClient.Entry b : c.buildings) if (b.code.equals(buildingId)) return;
        }
        throw HenuError.notFound("目录中没有该校区或教学楼");
    }

    private static String semesterFor(ConnectionRow row) {
        if (row == null || row.termYear == null || row.termTerm == null) return "";
        if ("0".equals(row.termTerm)) return row.termYear + "S2";
        if ("1".equals(row.termTerm)) return (Integer.parseInt(row.termYear) + 1) + "S1";
        return "";
    }
}
