package com.xique.assistant.henu.storage;

import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;

import com.xique.assistant.henu.ops.HenuError;

import org.json.JSONArray;
import org.json.JSONObject;

import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Calendar;
import java.util.Comparator;
import java.util.Iterator;
import java.util.List;
import java.util.Locale;
import java.util.TimeZone;

/**
 * Local SemesterSettings storage. The externally visible revision is always
 * the workspace aggregate revision; local_semesters.revision is only a
 * snapshot of the last write. PUT validates every field, compares the
 * normalized configuration for a true no-op and re-projects source-derived
 * courses in the same transaction.
 */
public final class LocalSettingsStore {

    public static final int PERIOD_COUNT = 13;
    public static final String TIME_ZONE = "Asia/Shanghai";
    public static final long MAX_SAFE_INTEGER = 9007199254740991L;

    private final LocalDb db;

    public LocalSettingsStore(Context context) {
        this(new LocalDb(context));
    }

    public LocalSettingsStore(LocalDb db) {
        if (db == null) throw new IllegalArgumentException("db");
        this.db = db;
    }

    public JSONObject get(String workspaceId, String semester) throws HenuError {
        String sem = requireSemester(semester);
        long revision;
        try {
            revision = db.workspaceRevision(workspaceId);
        } catch (RuntimeException e) {
            throw HenuError.notFound("工作区不存在");
        }
        JSONObject settings = readStored(db, workspaceId, sem);
        if (settings == null) settings = defaults(sem);
        try {
            settings.put("revision", revision);
        } catch (org.json.JSONException e) {
            throw HenuError.storage("设置序列化失败");
        }
        return envelope(revision, settings);
    }

    public JSONObject put(String workspaceId, long expectedRevision, JSONObject input)
            throws HenuError {
        if (expectedRevision < 0 || expectedRevision > MAX_SAFE_INTEGER) {
            throw HenuError.invalid("expected_revision 无效");
        }
        if (input == null) throw HenuError.invalid("settings 必填");
        String sem = requireSemester(input.optString("semester", ""));
        JSONObject normalized = normalize(input, sem);
        SQLiteDatabase writer = db.getWritableDatabase();
        writer.beginTransaction();
        try {
            long current;
            try {
                current = db.workspaceRevision(workspaceId);
            } catch (RuntimeException e) {
                throw HenuError.notFound("工作区不存在");
            }
            if (current != expectedRevision) throw new HenuError("REVISION_CONFLICT", "本机数据已变化");
            JSONObject old = readStored(db, workspaceId, sem);
            if (old != null && canonical(old).equals(canonical(normalized))) {
                writer.setTransactionSuccessful();
                JSONObject out = new JSONObject(normalized.toString());
                out.put("revision", current);
                return envelope(current, out);
            }

            // Once source-derived courses exist, incomplete times or an
            // unconfirmed anchor cannot be projected. Refuse instead of
            // silently deleting or freezing the existing timetable.
            if (hasSourceCourses(writer, workspaceId, sem) && !fullyConfigured(normalized)) {
                throw HenuError.invalid("已有来源课表，需保留完整作息与已确认锚点");
            }
            long now = System.currentTimeMillis();
            ContentValues v = new ContentValues();
            v.put("workspace_id", workspaceId);
            v.put("semester", sem);
            v.put("revision", current);
            v.put("settings_json", canonical(normalized));
            v.put("updated_at", now);
            writer.insertWithOnConflict("local_semesters", null, v,
                    SQLiteDatabase.CONFLICT_REPLACE);

            Projection.reprojectSemester(db, workspaceId, sem);

            long next = current + 1;
            ContentValues bump = new ContentValues();
            bump.put("revision", next);
            if (writer.update("local_workspaces", bump, "id=?", new String[]{workspaceId}) != 1) {
                throw HenuError.notFound("工作区不存在");
            }
            ContentValues rev = new ContentValues();
            rev.put("revision", next);
            writer.update("local_semesters", rev,
                    "workspace_id=? AND semester=?", new String[]{workspaceId, sem});
            writer.setTransactionSuccessful();
            JSONObject out = new JSONObject(normalized.toString());
            out.put("revision", next);
            return envelope(next, out);
        } catch (HenuError e) {
            throw e;
        } catch (org.json.JSONException e) {
            throw HenuError.storage("设置序列化失败");
        } catch (RuntimeException e) {
            throw HenuError.storage("设置保存失败");
        } finally {
            writer.endTransaction();
        }
    }

    public static JSONObject readStored(LocalDb db, String workspaceId, String semester)
            throws HenuError {
        try (Cursor c = db.getReadableDatabase().query("local_semesters",
                new String[]{"settings_json"}, "workspace_id=? AND semester=?",
                new String[]{workspaceId, semester}, null, null, null)) {
            if (!c.moveToFirst()) return null;
            JSONObject value = new JSONObject(c.getString(0));
            value.put("semester", semester);
            value.put("schema_version", 1);
            value.put("time_zone", TIME_ZONE);
            value.put("week_start_day", 1);
            value.put("period_count", PERIOD_COUNT);
            if (!value.has("revision")) value.put("revision", 0);
            return value;
        } catch (org.json.JSONException e) {
            throw HenuError.storage("学期设置损坏");
        }
    }

    public static JSONObject defaults(String semester) throws HenuError {
        String sem = requireSemester(semester);
        JSONObject out = new JSONObject();
        try {
            out.put("schema_version", 1);
            out.put("semester", sem);
            out.put("revision", 0);
            out.put("time_zone", TIME_ZONE);
            out.put("semester_label", JSONObject.NULL);
            out.put("week_anchor", new JSONObject()
                    .put("start_date", JSONObject.NULL)
                    .put("source", "UNKNOWN")
                    .put("confirmed", false));
            out.put("week_start_day", 1);
            out.put("total_weeks", JSONObject.NULL);
            out.put("period_count", PERIOD_COUNT);
            JSONArray periods = new JSONArray();
            for (int i = 1; i <= PERIOD_COUNT; i++) {
                periods.put(new JSONObject().put("index", i)
                        .put("start_time", JSONObject.NULL)
                        .put("end_time", JSONObject.NULL)
                        .put("configured", false));
            }
            out.put("periods", periods);
        } catch (org.json.JSONException e) {
            throw HenuError.storage("设置序列化失败");
        }
        return out;
    }

    public static boolean fullyConfigured(JSONObject settings) {
        if (settings == null) return false;
        if (settings.isNull("semester_label") || settings.optString("semester_label").isEmpty()
                || settings.isNull("total_weeks") || settings.optInt("total_weeks",0) < 1) return false;
        JSONObject anchor = settings.optJSONObject("week_anchor");
        if (anchor == null || !anchor.optBoolean("confirmed", false)
                || anchor.isNull("start_date") || anchor.optString("start_date", "").isEmpty()) {
            return false;
        }
        JSONArray periods = settings.optJSONArray("periods");
        if (periods == null || periods.length() != PERIOD_COUNT) return false;
        for (int i = 0; i < periods.length(); i++) {
            JSONObject p = periods.optJSONObject(i);
            if (p == null || !p.optBoolean("configured", false)
                    || p.isNull("start_time") || p.isNull("end_time")) return false;
        }
        return true;
    }

    /** Strict normalization of the TS SemesterSettings payload. */
    public static JSONObject normalize(JSONObject input, String semester) throws HenuError {
        try {
            return normalizeChecked(input, semester);
        } catch (org.json.JSONException e) {
            throw HenuError.storage("设置序列化失败");
        }
    }

    private static JSONObject normalizeChecked(JSONObject input, String semester)
            throws HenuError, org.json.JSONException {
        String sem = requireSemester(semester);
        rejectUnknown(input, new String[]{"schema_version", "semester", "revision", "time_zone",
                "semester_label", "week_anchor", "week_start_day", "total_weeks",
                "period_count", "periods", "campus"});
        if (!sem.equals(input.optString("semester", ""))) throw HenuError.invalid("semester 不一致");
        if (input.has("schema_version") && !input.isNull("schema_version")
                && integer(input.opt("schema_version"), 1, 1, "schema_version") != 1) {
            throw HenuError.invalid("schema_version 不支持");
        }
        if (input.has("revision") && !input.isNull("revision")) {
            integer(input.opt("revision"), 0, MAX_SAFE_INTEGER, "revision");
        }
        if (input.has("time_zone") && !input.isNull("time_zone")
                && !TIME_ZONE.equals(input.opt("time_zone"))) {
            throw HenuError.invalid("time_zone 必须为 Asia/Shanghai");
        }
        if (input.has("week_start_day") && !input.isNull("week_start_day")
                && integer(input.opt("week_start_day"), 1, 1, "week_start_day") != 1) {
            throw HenuError.invalid("week_start_day 必须为周一");
        }
        if (!input.has("semester_label")) throw HenuError.invalid("semester_label 必填");
        Object labelRaw = input.opt("semester_label");
        String label = null;
        if (labelRaw == null || labelRaw == JSONObject.NULL) {
            label = null;
        } else if (labelRaw instanceof String) {
            String trimmed = ((String) labelRaw).trim();
            if (!trimmed.isEmpty()) label = trimmed;
        } else {
            throw HenuError.invalid("semester_label 类型无效");
        }
        if (label != null) {
            if (label.length() > 128 || !label.matches("[0-9]{4}-[0-9]{4}学年(第一|第二)学期")) {
                throw HenuError.invalid("semester_label 无效");
            }
            int first = Integer.parseInt(label.substring(0, 4));
            int second = Integer.parseInt(label.substring(5, 9));
            if (second != first + 1) throw HenuError.invalid("semester_label 学年无效");
        }

        if (!input.has("week_anchor") || input.isNull("week_anchor")
                || input.optJSONObject("week_anchor") == null) {
            throw HenuError.invalid("week_anchor 必填");
        }
        JSONObject anchorIn = input.getJSONObject("week_anchor");
        rejectUnknown(anchorIn, new String[]{"start_date", "source", "confirmed"});
        if (!anchorIn.has("confirmed") || !(anchorIn.opt("confirmed") instanceof Boolean)) {
            throw HenuError.invalid("week_anchor.confirmed 必填");
        }
        boolean confirmed = anchorIn.optBoolean("confirmed");
        Object dateRaw = anchorIn.opt("start_date");
        String anchor = null;
        if (dateRaw instanceof String && !((String) dateRaw).trim().isEmpty()) {
            anchor = ((String) dateRaw).trim();
        } else if (dateRaw != null && dateRaw != JSONObject.NULL && !(dateRaw instanceof String)) {
            throw HenuError.invalid("week_anchor.start_date 类型无效");
        }
        String source = anchorIn.optString("source", "");
        if (anchor == null) {
            if (confirmed || (!source.isEmpty() && !"UNKNOWN".equals(source))) {
                throw HenuError.invalid("未填写锚点时不能确认");
            }
            source = "UNKNOWN";
        } else {
            if (!validMonday(anchor)) throw HenuError.invalid("week_anchor.start_date 必须是周一");
            if (!"USER".equals(source) || !confirmed) {
                throw HenuError.invalid("已填写锚点必须是 USER 且 confirmed=true");
            }
        }

        if (!input.has("total_weeks")) throw HenuError.invalid("total_weeks 必填");
        Integer totalWeeks = null;
        if (!input.isNull("total_weeks")) {
            totalWeeks = (int) integer(input.opt("total_weeks"), 1, 60, "total_weeks");
        }
        if (!input.has("period_count")) throw HenuError.invalid("period_count 必填");
        int count = (int) integer(input.opt("period_count"), PERIOD_COUNT, PERIOD_COUNT, "period_count");
        if (!input.has("periods") || input.optJSONArray("periods") == null) {
            throw HenuError.invalid("periods 必填");
        }
        JSONArray periodsIn = input.getJSONArray("periods");
        if (periodsIn.length() != count) throw HenuError.invalid("periods 必须包含 13 节");
        List<JSONObject> periods = new ArrayList<>();
        for (int i = 0; i < count; i++) {
            JSONObject p = periodsIn.optJSONObject(i);
            if (p == null) throw HenuError.invalid("periods 条目损坏");
            rejectUnknown(p, new String[]{"index", "start_time", "end_time", "configured"});
            int index = (int) integer(p.opt("index"), 1, count, "index");
            if (index != i + 1) throw HenuError.invalid("节次必须按 1–13 连续排列");
            if (!p.has("configured") || !(p.opt("configured") instanceof Boolean)) {
                throw HenuError.invalid("configured 必填");
            }
            boolean configured = p.optBoolean("configured");
            String start = nullableString(p.opt("start_time"), "start_time");
            String end = nullableString(p.opt("end_time"), "end_time");
            if (!configured) {
                if (start != null || end != null) throw HenuError.invalid("未配置节次不能带时刻");
            } else {
                if (start == null || end == null) throw HenuError.invalid("已配置节次必须有起止时刻");
                if (!clock(start) || !clock(end) || start.compareTo(end) >= 0) {
                    throw HenuError.invalid("节次时刻无效");
                }
            }
            periods.add(new JSONObject().put("index", index)
                    .put("start_time", start == null ? JSONObject.NULL : start)
                    .put("end_time", end == null ? JSONObject.NULL : end)
                    .put("configured", configured));
        }
        periods.sort(Comparator.comparingInt(o -> o.optInt("index")));
        JSONObject previous = null;
        for (JSONObject p : periods) {
            if (!p.optBoolean("configured")) continue;
            if (previous != null && p.getString("start_time").compareTo(previous.getString("end_time")) < 0) {
                throw HenuError.invalid("节次时间重叠或倒序");
            }
            previous = p;
        }

        JSONObject out = new JSONObject();
        out.put("schema_version", 1);
        if (input.has("campus") && !input.isNull("campus")) {
            Object campus = input.opt("campus");
            if (!(campus instanceof String) || !("郑州".equals(campus) || "金明".equals(campus) || "明伦".equals(campus)))
                throw HenuError.invalid("校区无效");
            out.put("campus", campus);
        }
        out.put("semester", sem);
        out.put("revision", 0);
        out.put("time_zone", TIME_ZONE);
        out.put("semester_label", label == null ? JSONObject.NULL : label);
        out.put("week_anchor", new JSONObject()
                .put("start_date", anchor == null ? JSONObject.NULL : anchor)
                .put("source", source)
                .put("confirmed", confirmed));
        out.put("week_start_day", 1);
        out.put("total_weeks", totalWeeks == null ? JSONObject.NULL : totalWeeks);
        out.put("period_count", count);
        JSONArray normalizedPeriods = new JSONArray();
        for (JSONObject p : periods) normalizedPeriods.put(p);
        out.put("periods", normalizedPeriods);
        return out;
    }

    private static JSONObject envelope(long revision, JSONObject settings) throws HenuError {
        try {
            return new JSONObject().put("revision", revision).put("settings", settings);
        } catch (org.json.JSONException e) {
            throw HenuError.storage("设置序列化失败");
        }
    }

    private static boolean hasSourceCourses(SQLiteDatabase db, String ws, String semester) {
        try (Cursor c = db.rawQuery("SELECT count(*) FROM local_courses WHERE workspace_id=?"
                + " AND semester=? AND active=1 AND source_kind='HENU'",
                new String[]{ws, semester})) {
            return c.moveToFirst() && c.getLong(0) > 0;
        }
    }

    private static String requireSemester(String semester) throws HenuError {
        if (semester == null || !semester.matches("[A-Za-z0-9_-]{1,64}") || "current".equals(semester)) {
            throw HenuError.invalid("semester 不合法");
        }
        return semester;
    }

    private static void rejectUnknown(JSONObject object, String[] allowed) throws HenuError {
        Iterator<String> keys = object.keys();
        java.util.Set<String> set = new java.util.HashSet<>();
        for (String a : allowed) set.add(a);
        while (keys.hasNext()) if (!set.contains(keys.next())) throw HenuError.invalid("未知字段");
    }

    private static long integer(Object value, long min, long max, String name) throws HenuError {
        if (!(value instanceof Number)) throw HenuError.invalid(name + " 类型无效");
        Number n = (Number) value;
        double d = n.doubleValue();
        long l = n.longValue();
        if (!Double.isFinite(d) || d != (double) l || l < min || l > max) {
            throw HenuError.invalid(name + " 超出范围");
        }
        return l;
    }

    private static String nullableString(Object value, String name) throws HenuError {
        if (value == null || value == JSONObject.NULL) return null;
        if (!(value instanceof String)) throw HenuError.invalid(name + " 类型无效");
        String s = ((String) value).trim();
        return s.isEmpty() ? null : s;
    }

    private static boolean clock(String value) {
        return value.matches("(?:[01][0-9]|2[0-3]):[0-5][0-9]");
    }

    private static boolean validMonday(String value) {
        if (!value.matches("[0-9]{4}-[0-9]{2}-[0-9]{2}")) return false;
        try {
            SimpleDateFormat f = new SimpleDateFormat("yyyy-MM-dd", Locale.ROOT);
            f.setLenient(false);
            f.setTimeZone(TimeZone.getTimeZone(TIME_ZONE));
            java.util.Date d = f.parse(value);
            if (d == null) return false;
            Calendar c = Calendar.getInstance(TimeZone.getTimeZone(TIME_ZONE), Locale.ROOT);
            c.setTime(d);
            return c.get(Calendar.DAY_OF_WEEK) == Calendar.MONDAY;
        } catch (Exception e) {
            return false;
        }
    }

    /** Deterministic key-sorted serialization for semantic no-op checks. */
    public static String canonical(JSONObject value) throws HenuError {
        try {
            return canonicalValue(value).toString();
        } catch (org.json.JSONException e) {
            throw HenuError.storage("设置序列化失败");
        }
    }

    private static Object canonicalValue(Object value) throws org.json.JSONException {
        if (value instanceof JSONObject) {
            JSONObject object = (JSONObject) value;
            java.util.TreeMap<String, Object> sorted = new java.util.TreeMap<>();
            Iterator<String> keys = object.keys();
            while (keys.hasNext()) {
                String key = keys.next();
                if ("revision".equals(key)) continue;
                sorted.put(key, canonicalValue(object.get(key)));
            }
            JSONObject out = new JSONObject();
            for (java.util.Map.Entry<String, Object> e : sorted.entrySet()) out.put(e.getKey(), e.getValue());
            return out;
        }
        if (value instanceof JSONArray) {
            JSONArray in = (JSONArray) value;
            JSONArray out = new JSONArray();
            for (int i = 0; i < in.length(); i++) out.put(canonicalValue(in.get(i)));
            return out;
        }
        return value;
    }
}
