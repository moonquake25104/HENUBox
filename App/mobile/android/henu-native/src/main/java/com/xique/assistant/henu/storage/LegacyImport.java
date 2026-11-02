package com.xique.assistant.henu.storage;

import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;

import com.xique.assistant.henu.ops.HenuError;

import org.json.JSONArray;
import org.json.JSONObject;

import java.text.SimpleDateFormat;
import java.util.HashSet;
import java.util.Locale;
import java.util.Set;
import java.util.TimeZone;
import java.util.UUID;

/**
 * One-way, idempotent import of the main app's legacy v1 timetable snapshot
 * (schema 1 Course[]). The old file is never modified. A marker and every
 * copied row are written in one SQLite transaction; an empty or malformed
 * snapshot never creates the marker.
 */
public final class LegacyImport {

    public static final String MIGRATION_NAME = "legacy-v1";
    public static final String SOURCE_KIND = "LEGACY_UNVERIFIED";

    private LegacyImport() {
    }

    /** Entry point used by the main app after SessionVault + TimetableStore
     * returned a decrypted snapshot for the current binding. */
    public static boolean importSnapshot(Context context, JSONObject snapshot, long generation)
            throws HenuError {
        if (context == null) throw HenuError.invalid("缺少上下文");
        LocalDb db = new LocalDb(context);
        try {
            return importSnapshot(db, snapshot, generation);
        } finally {
            db.close();
        }
    }

    public static boolean importSnapshot(LocalDb db, JSONObject snapshot, long generation)
            throws HenuError {
        if (generation < 0 || generation > 9007199254740991L) throw HenuError.invalid("代际无效");
        JSONArray courses = validate(snapshot);
        if (courses.length() == 0) return false;

        String ws = db.ensureWorkspace();
        SQLiteDatabase writer = db.getWritableDatabase();
        writer.beginTransaction();
        try {
            if (hasMigration(writer, MIGRATION_NAME)) {
                writer.setTransactionSuccessful();
                return false;
            }
            try (Cursor c = writer.rawQuery(
                    "SELECT count(*) FROM local_courses WHERE workspace_id=?", new String[]{ws})) {
                if (c.moveToFirst() && c.getLong(0) != 0) {
                    throw HenuError.storage("本机课表已存在，旧课表未覆盖");
                }
            }

            long maxId = 0;
            long now = System.currentTimeMillis();
            for (int i = 0; i < courses.length(); i++) {
                JSONObject course = courses.getJSONObject(i);
                long id = course.getLong("id");
                maxId = Math.max(maxId, id);
                ContentValues v = new ContentValues();
                v.put("workspace_id", ws);
                v.put("id", id);
                v.put("semester", course.getString("semester"));
                v.put("sync_id", course.getString("sync_id").toLowerCase(Locale.ROOT));
                v.put("name", course.getString("name"));
                v.put("day_of_week", course.getInt("day_of_week"));
                v.put("start_time", course.getString("start_time"));
                v.put("end_time", course.getString("end_time"));
                v.put("week_pattern", course.getString("week_pattern"));
                v.put("semester_start_date", course.getString("semester_start_date"));
                v.put("location", course.getString("location"));
                v.put("teacher", course.getString("teacher"));
                v.put("color", course.getString("color"));
                v.put("source_kind", SOURCE_KIND);
                v.putNull("source_entry_id");
                v.putNull("source_course_id");
                v.put("active", 1);
                v.put("created_at", course.getString("created_at"));
                v.put("updated_at", course.getString("updated_at"));
                writer.insertOrThrow("local_courses", null, v);
            }
            ContentValues next = new ContentValues();
            next.put("next_course_id", maxId + 1);
            if (writer.update("local_workspaces", next, "id=?", new String[]{ws}) != 1) {
                throw HenuError.storage("工作区不存在");
            }
            ContentValues marker = new ContentValues();
            marker.put("name", MIGRATION_NAME);
            marker.put("completed_at", now);
            writer.insertOrThrow("local_migrations", null, marker);
            writer.setTransactionSuccessful();
            return true;
        } catch (HenuError e) {
            throw e;
        } catch (Exception e) {
            throw HenuError.storage("旧课表写入失败");
        } finally {
            writer.endTransaction();
        }
    }

    public static boolean isComplete(LocalDb db) {
        try (Cursor c = db.getReadableDatabase().query("local_migrations", new String[]{"name"},
                "name=?", new String[]{MIGRATION_NAME}, null, null, null)) {
            return c.moveToFirst();
        }
    }

    private static boolean hasMigration(SQLiteDatabase db, String name) {
        try (Cursor c = db.query("local_migrations", new String[]{"name"},
                "name=?", new String[]{name}, null, null, null)) {
            return c.moveToFirst();
        }
    }

    /** Validates the exact legacy TimetableSnapshot shape. Throws for every
     * structural or field problem; checks happen before any write. */
    private static JSONArray validate(JSONObject snapshot) throws HenuError {
        if (snapshot == null) return new JSONArray();
        if (snapshot.optInt("schema", -1) != 1) throw HenuError.invalid("旧课表版本不支持");
        String savedAt = snapshot.optString("saved_at", "");
        if (!validIso(savedAt)) throw HenuError.invalid("旧课表保存时间无效");
        JSONArray courses = snapshot.optJSONArray("courses");
        if (courses == null) throw HenuError.invalid("旧课表内容缺失");
        if (courses.length() > 10000) throw HenuError.invalid("旧课表条目过多");
        Set<Long> ids = new HashSet<>();
        Set<String> syncs = new HashSet<>();
        for (int i = 0; i < courses.length(); i++) {
            JSONObject c = courses.optJSONObject(i);
            if (c == null) throw HenuError.invalid("旧课表条目损坏");
            long id = integer(c, "id", 1, 9007199254740991L);
            if (!ids.add(id)) throw HenuError.invalid("旧课表 ID 重复");
            String sync = text(c, "sync_id", 1, 64);
            try {
                UUID.fromString(sync);
            } catch (IllegalArgumentException e) {
                throw HenuError.invalid("旧课表 sync_id 无效");
            }
            if (!syncs.add(sync.toLowerCase(Locale.ROOT))) throw HenuError.invalid("旧课表 sync_id 重复");
            text(c, "name", 1, 512);
            text(c, "semester", 1, 128);
            text(c, "week_pattern", 0, 512);
            text(c, "teacher", 0, 512);
            text(c, "location", 0, 512);
            text(c, "color", 1, 32);
            String start = text(c, "start_time", 5, 5);
            String end = text(c, "end_time", 5, 5);
            if (!clock(start) || !clock(end) || start.compareTo(end) >= 0) {
                throw HenuError.invalid("旧课表时间无效");
            }
            integer(c, "day_of_week", 1, 7);
            String anchor = text(c, "semester_start_date", 10, 10);
            if (!validDate(anchor)) throw HenuError.invalid("旧课表学期日期无效");
            text(c, "created_at", 1, 64);
            text(c, "updated_at", 1, 64);
            if (c.has("deleted_at") && !c.isNull("deleted_at")) {
                throw HenuError.invalid("旧课表包含已删除条目");
            }
        }
        return courses;
    }

    private static String text(JSONObject o, String key, int min, int max) throws HenuError {
        if (!o.has(key) || o.isNull(key)) throw HenuError.invalid("旧课表字段缺失");
        Object value = o.opt(key);
        if (!(value instanceof String)) throw HenuError.invalid("旧课表字段类型无效");
        String s = (String) value;
        if (s.length() < min || s.length() > max || s.indexOf('\0') >= 0) {
            throw HenuError.invalid("旧课表字段长度无效");
        }
        return s;
    }

    private static long integer(JSONObject o, String key, long min, long max) throws HenuError {
        Object value = o.opt(key);
        if (!(value instanceof Number)) throw HenuError.invalid("旧课表数字字段无效");
        double d = ((Number) value).doubleValue();
        long l = ((Number) value).longValue();
        if (!Double.isFinite(d) || d != (double) l || l < min || l > max) {
            throw HenuError.invalid("旧课表数字字段无效");
        }
        return l;
    }

    private static boolean clock(String s) {
        return s.matches("(?:[01][0-9]|2[0-3]):[0-5][0-9]");
    }

    private static boolean validDate(String s) {
        if (!s.matches("[0-9]{4}-[0-9]{2}-[0-9]{2}")) return false;
        try {
            SimpleDateFormat f = new SimpleDateFormat("yyyy-MM-dd", Locale.ROOT);
            f.setLenient(false);
            f.setTimeZone(TimeZone.getTimeZone("Asia/Shanghai"));
            return f.parse(s) != null;
        } catch (Exception e) {
            return false;
        }
    }

    private static boolean validIso(String s) {
        if (s == null || s.length() < 20 || s.length() > 40) return false;
        return s.matches("[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\\.[0-9]{1,9})?(?:Z|[+-][0-9]{2}:[0-9]{2})");
    }
}
