package com.xique.assistant.henu.storage;

import android.content.ContentValues;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;

import com.xique.assistant.henu.ops.HenuError;

import org.json.JSONArray;
import org.json.JSONObject;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Date;
import java.util.HashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.TimeZone;
import java.util.UUID;

/**
 * Pure-ish projection helpers plus the settings -> local_courses re-projection
 * used by PUT settings. A local course is one display row: a source session
 * with non-contiguous sections is split into one row per contiguous section
 * segment. Times are interpreted on the fixed Asia/Shanghai clock.
 */
public final class Projection {

    public static final String SOURCE_HENU = "HENU";
    public static final String SOURCE_LEGACY = LegacyImport.SOURCE_KIND;
    public static final String SOURCE_MANUAL = "MANUAL";
    public static final String DEFAULT_COLOR = "#7C8CC0";

    private Projection() {
    }

    public static final class Segment {
        public final int ordinal;
        public final int first;
        public final int last;

        Segment(int ordinal, int first, int last) {
            this.ordinal = ordinal;
            this.first = first;
            this.last = last;
        }
    }

    public static List<Segment> segments(JSONArray sections) throws HenuError {
        int[] values = sortedInts(sections, 1, LocalSettingsStore.PERIOD_COUNT, "sections");
        if (values.length == 0) throw HenuError.invalid("节次不能为空");
        List<Segment> out = new ArrayList<>();
        int first = values[0], last = values[0];
        for (int i = 1; i < values.length; i++) {
            if (values[i] == last + 1) {
                last = values[i];
            } else {
                out.add(new Segment(out.size(), first, last));
                first = last = values[i];
            }
        }
        out.add(new Segment(out.size(), first, last));
        return out;
    }

    public static int[] sortedInts(JSONArray array, int min, int max, String name)
            throws HenuError {
        if (array == null) throw HenuError.invalid(name + " 缺失");
        List<Integer> values = new ArrayList<>();
        for (int i = 0; i < array.length(); i++) {
            Object raw = array.opt(i);
            if (!(raw instanceof Number)) throw HenuError.invalid(name + " 类型无效");
            Number n = (Number) raw;
            double d = n.doubleValue();
            int v = n.intValue();
            if (!Double.isFinite(d) || d != (double) v || v < min || v > max) {
                throw HenuError.invalid(name + " 超出范围");
            }
            values.add(v);
        }
        Collections.sort(values);
        LinkedHashSet<Integer> unique = new LinkedHashSet<>(values);
        int[] out = new int[unique.size()];
        int i = 0;
        for (Integer v : unique) out[i++] = v;
        return out;
    }

    public static String weekPattern(JSONArray weeks) throws HenuError {
        int[] values = sortedInts(weeks, 1, 60, "weeks");
        if (values.length == 0) throw HenuError.invalid("周次不能为空");
        StringBuilder out = new StringBuilder();
        for (int i = 0; i < values.length; i++) {
            if (i > 0) out.append(',');
            out.append(values[i]).append('周');
        }
        return out.toString();
    }

    /** Returns {start,end} in Asia/Shanghai wall time, or null when any
     * requested period is unknown/unconfigured. Never guesses from neighbours. */
    public static String[] times(JSONObject settings, JSONArray sections) throws HenuError {
        int[] values = sortedInts(sections, 1, LocalSettingsStore.PERIOD_COUNT, "sections");
        if (values.length == 0) return null;
        JSONArray periods = settings == null ? null : settings.optJSONArray("periods");
        if (periods == null) return null;
        Map<Integer, String[]> byIndex = new HashMap<>();
        for (int i = 0; i < periods.length(); i++) {
            JSONObject p = periods.optJSONObject(i);
            if (p == null || !p.optBoolean("configured", false)
                    || p.isNull("start_time") || p.isNull("end_time")) continue;
            byIndex.put(p.optInt("index"), new String[]{
                    p.optString("start_time"), p.optString("end_time")});
        }
        String[] first = byIndex.get(values[0]);
        String[] last = byIndex.get(values[values.length - 1]);
        if (first == null || last == null) return null;
        for (int value : values) if (!byIndex.containsKey(value)) return null;
        return new String[]{first[0], last[1]};
    }

    public static String anchor(JSONObject settings) {
        if (settings == null) return "";
        JSONObject weekAnchor = settings.optJSONObject("week_anchor");
        if (weekAnchor == null || !weekAnchor.optBoolean("confirmed", false)) return "";
        String value = weekAnchor.optString("start_date", "");
        return value.matches("[0-9]{4}-[0-9]{2}-[0-9]{2}") ? value : "";
    }

    public static String courseKey(JSONObject course) {
        return sha256("course\n"
                + clean(course.optString("name", "")) + "\n"
                + nullable(course, "upstream_course_code") + "\n"
                + nullable(course, "teaching_class_code") + "\n"
                + nullable(course, "credits"));
    }

    public static String courseKeyFromSource(JSONObject source) {
        return sha256("course\n"
                + clean(source.optString("course_name", source.optString("name", ""))) + "\n"
                + nullable(source, "upstream_course_code") + "\n"
                + nullable(source, "teaching_class_code") + "\n"
                + nullable(source, "credits"));
    }

    public static String sessionHash(JSONObject session) throws HenuError {
        return sessionHash(session.optInt("weekday", 0), session.optJSONArray("sections"),
                session.optJSONArray("weeks"), session.optString("location", ""),
                session.optString("teacher", ""));
    }

    public static String contentHash(JSONObject course, JSONObject session) throws HenuError {
        return sessionHash(session.optInt("weekday"),session.optJSONArray("sections"),session.optJSONArray("weeks"),
            session.optString("location"),teacher(course,session));
    }

    public static String identity(String semester, JSONObject course, JSONObject session) throws HenuError {
        return semester + ":" + courseKey(course) + ":" + contentHash(course,session);
    }

    public static String storedIdentity(LocalDb db, String ws, String semester, String sessionId, JSONObject source) throws HenuError {
        String sourceSemester = source.optString("semester", "");
        if (sourceSemester.isEmpty()) {
            try (Cursor c = db.getReadableDatabase().query("local_courses",new String[]{"semester"},
                    "workspace_id=? AND source_entry_id LIKE ?",new String[]{ws,sessionId+"#%"},null,null,null)) {
                if (!c.moveToFirst()) throw HenuError.storage("课表来源学期缺失");
                sourceSemester=c.getString(0);
            }
        }
        if (!semester.equals(sourceSemester)) return null;
        return semester + ":" + courseKeyFromSource(source) + ":" + sessionHash(source);
    }

    public static String sessionHash(int weekday, JSONArray sections, JSONArray weeks,
                                     String location, String teacher) throws HenuError {
        if (weekday < 1 || weekday > 7) throw HenuError.invalid("weekday 无效");
        int[] s = sortedInts(sections, 1, LocalSettingsStore.PERIOD_COUNT, "sections");
        int[] w = sortedInts(weeks, 1, 60, "weeks");
        StringBuilder raw = new StringBuilder("session\n").append(weekday).append('\n');
        appendInts(raw, s);
        appendInts(raw, w);
        raw.append(clean(location)).append('\n').append(clean(teacher));
        return sha256(raw.toString());
    }

    public static Set<Integer> weekSet(JSONArray weeks) throws HenuError {
        int[] values = sortedInts(weeks, 1, 60, "weeks");
        Set<Integer> out = new LinkedHashSet<>();
        for (int v : values) out.add(v);
        return out;
    }

    /** Parses the legacy week-pattern text with the same segment semantics as
     * the existing calendar matcher. Only 1..60 is considered. */
    public static Set<Integer> expandWeekPattern(String pattern, int totalWeeks) {
        Set<Integer> out = new LinkedHashSet<>();
        String text = pattern == null ? "" : pattern.replaceAll(" ", "");
        int max = totalWeeks >= 1 && totalWeeks <= 60 ? totalWeeks : 60;
        if (text.isEmpty()) {
            for (int i = 1; i <= max; i++) out.add(i);
            return out;
        }
        for (String segment : text.split("[,，;；、/]")) {
            if (segment.isEmpty()) continue;
            int pos = segment.indexOf('周');
            String range = (pos < 0 ? segment : segment.substring(0, pos)).replaceFirst("^第", "");
            String tail = pos < 0 ? "" : segment.substring(pos + 1);
            int start;
            int end;
            if ("单".equals(range) || "双".equals(range) || "全".equals(range)) {
                start = 1;
                end = max;
            } else {
                java.util.regex.Matcher m = java.util.regex.Pattern
                        .compile("^(\\d+)(?:-(\\d+))?$").matcher(range);
                if (!m.matches()) continue;
                start = Integer.parseInt(m.group(1));
                end = m.group(2) == null ? start : Integer.parseInt(m.group(2));
            }
            if (start > end) continue;
            for (int week = Math.max(1, start); week <= Math.min(max, end); week++) {
                boolean odd = "单".equals(range) || tail.contains("单");
                boolean even = "双".equals(range) || tail.contains("双");
                if (odd && week % 2 == 0) continue;
                if (even && week % 2 != 0) continue;
                out.add(week);
            }
        }
        return out;
    }

    public static String legacyMatchKey(String name, int weekday, String start, String end,
                                        String pattern, int totalWeeks) {
        Set<Integer> weeks = expandWeekPattern(pattern, totalWeeks);
        if (weeks.isEmpty()) return "";
        StringBuilder key = new StringBuilder("legacy\n")
                .append(clean(name)).append('\n').append(weekday).append('\n')
                .append(clean(start)).append('\n').append(clean(end)).append('\n');
        for (Integer week : weeks) key.append(week).append(',');
        return key.toString();
    }

    public static String previewMatchKey(JSONObject previewCourse, JSONObject session,
                                         int totalWeeks) throws HenuError {
        String[] time = timesFromSession(session);
        if (time == null) return "";
        return legacyMatchKey(previewCourse.optString("name", ""), session.optInt("weekday", 0),
                time[0], time[1],
                weekPattern(session.optJSONArray("weeks")), totalWeeks);
    }

    private static String[] timesFromSession(JSONObject session) {
        if (session.isNull("start_time") || session.isNull("end_time")) return null;
        String start = session.optString("start_time", "");
        String end = session.optString("end_time", "");
        return start.isEmpty() || end.isEmpty() ? null : new String[]{start, end};
    }

    public static String entryKey(String sessionId, int segmentOrdinal) {
        return sessionId + "#" + segmentOrdinal;
    }

    public static String entrySessionId(String entryKey) {
        int at = entryKey == null ? -1 : entryKey.lastIndexOf('#');
        return at <= 0 ? null : entryKey.substring(0, at);
    }

    public static int entryOrdinal(String entryKey, int fallback) {
        int at = entryKey == null ? -1 : entryKey.lastIndexOf('#');
        if (at < 0) return fallback;
        try {
            return Integer.parseInt(entryKey.substring(at + 1));
        } catch (NumberFormatException e) {
            return fallback;
        }
    }

    public static String stableSyncId(String ws, String sessionId, int segmentOrdinal) {
        return UUID.nameUUIDFromBytes(("xique-local-course\n" + ws + "\n" + sessionId + "\n"
                + segmentOrdinal).getBytes(StandardCharsets.UTF_8)).toString().toLowerCase(Locale.ROOT);
    }

    /** Updates every HENU-derived row after settings change. Caller owns the
     * transaction; any failure propagates and rolls the settings update back. */
    public static void reprojectSemester(LocalDb db, String ws, String semester) throws HenuError {
        JSONObject settings = LocalSettingsStore.readStored(db, ws, semester);
        if (settings == null) return;
        SQLiteDatabase writer = db.getWritableDatabase();
        List<Row> rows = new ArrayList<>();
        try (Cursor c = writer.query("local_courses",
                new String[]{"id", "source_entry_id", "day_of_week"},
                "workspace_id=? AND semester=? AND active=1 AND source_kind='HENU'"
                        + " AND source_entry_id IS NOT NULL",
                new String[]{ws, semester}, null, null, "id")) {
            while (c.moveToNext()) {
                rows.add(new Row(c.getLong(0), c.getString(1)));
            }
        }
        if (rows.isEmpty()) return;
        if (!LocalSettingsStore.fullyConfigured(settings)) {
            throw HenuError.invalid("作息不完整，无法重投影");
        }
        String anchor = anchor(settings);
        if (anchor.isEmpty()) throw HenuError.invalid("教学周锚点未确认");
        Map<String, JSONObject> sources = new HashMap<>();
        long now = System.currentTimeMillis();
        String nowIso = iso(now);
        for (Row row : rows) {
            String sessionId = entrySessionId(row.entryKey);
            if (sessionId == null) throw HenuError.storage("课表来源关联损坏");
            JSONObject source = sources.get(sessionId);
            if (source == null) {
                source = readSourceSession(writer, ws, sessionId);
                if (source == null) throw HenuError.storage("课表来源记录缺失");
                sources.put(sessionId, source);
            }
            List<Segment> segments = segments(source.optJSONArray("sections"));
            int ordinal = entryOrdinal(row.entryKey, 0);
            if (ordinal < 0 || ordinal >= segments.size()) {
                throw HenuError.storage("课表来源分段损坏");
            }
            String[] time = times(settings, toArray(segments.get(ordinal)));
            if (time == null) throw HenuError.invalid("作息缺少所需节次");
            String pattern = weekPattern(source.optJSONArray("weeks"));
            ContentValues v = new ContentValues();
            v.put("start_time", time[0]);
            v.put("end_time", time[1]);
            v.put("week_pattern", pattern);
            v.put("semester_start_date", anchor);
            // Personal structural edits take precedence over later campus
            // re-projections. Never rewrite the immutable school source.
            JSONObject personal = LocalCourseStore.overrides(writer, ws, row.id);
            for (String field : new String[]{"start_time", "end_time", "week_pattern", "semester_start_date"}) {
                if (personal.has(field)) v.remove(field);
            }
            String effectiveStart = personal.optString("start_time", time[0]);
            String effectiveEnd = personal.optString("end_time", time[1]);
            if (effectiveStart.compareTo(effectiveEnd) >= 0) throw HenuError.invalid("新校区作息与个人上课时间冲突，请先调整课程时间");
            v.put("updated_at", nowIso);
            writer.update("local_courses", v, "workspace_id=? AND id=?",
                    new String[]{ws, String.valueOf(row.id)});
        }
    }

    public static JSONObject readSourceSession(SQLiteDatabase db, String ws, String sessionId) {
        try (Cursor c = db.query("local_source_sessions", new String[]{"source_json"},
                "workspace_id=? AND session_id=? AND active=1", new String[]{ws, sessionId},
                null, null, null)) {
            if (!c.moveToFirst()) return null;
            return new JSONObject(c.getString(0));
        } catch (org.json.JSONException e) {
            return null;
        }
    }

    private static JSONArray toArray(Segment segment) {
        JSONArray out = new JSONArray();
        for (int i = segment.first; i <= segment.last; i++) out.put(i);
        return out;
    }

    public static JSONArray copySections(JSONArray sections) throws HenuError {
        int[] values = sortedInts(sections, 1, LocalSettingsStore.PERIOD_COUNT, "sections");
        JSONArray out = new JSONArray();
        for (int v : values) out.put(v);
        return out;
    }

    public static JSONArray copyWeeks(JSONArray weeks) throws HenuError {
        int[] values = sortedInts(weeks, 1, 60, "weeks");
        JSONArray out = new JSONArray();
        for (int v : values) out.put(v);
        return out;
    }

    public static String teacher(JSONObject course, JSONObject session) {
        String teacher = clean(session.optString("teacher", ""));
        if (!teacher.isEmpty()) return teacher;
        JSONArray teachers = course.optJSONArray("teachers");
        if (teachers == null) return "";
        StringBuilder out = new StringBuilder();
        for (int i = 0; i < teachers.length(); i++) {
            String value = clean(teachers.optString(i, ""));
            if (value.isEmpty()) continue;
            if (out.length() > 0) out.append('、');
            out.append(value);
        }
        return out.toString();
    }

    public static String iso(long millis) {
        SimpleDateFormat f = new SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.ROOT);
        f.setTimeZone(TimeZone.getTimeZone("UTC"));
        return f.format(new Date(millis));
    }

    private static String nullable(JSONObject object, String key) {
        Object value = object.opt(key);
        return value == null || value == JSONObject.NULL ? "" : clean(String.valueOf(value));
    }

    private static String clean(String value) {
        return value == null ? "" : value.trim();
    }

    private static void appendInts(StringBuilder out, int[] values) {
        for (int value : values) out.append(value).append(',');
        out.append('\n');
    }

    public static String sha256(String value) {
        try {
            MessageDigest md = MessageDigest.getInstance("SHA-256");
            byte[] digest = md.digest(value.getBytes(StandardCharsets.UTF_8));
            StringBuilder out = new StringBuilder();
            for (byte b : digest) out.append(String.format("%02x", b));
            return out.toString();
        } catch (Exception e) {
            throw new IllegalStateException("sha-256 unavailable");
        }
    }

    private static final class Row {
        final long id;
        final String entryKey;

        Row(long id, String entryKey) {
            this.id = id;
            this.entryKey = entryKey;
        }
    }
}
