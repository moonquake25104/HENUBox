package com.xique.assistant.henu.storage;

import android.content.ContentValues;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;
import com.xique.assistant.henu.ops.HenuError;
import org.json.JSONObject;
import java.time.LocalDate;
import java.util.Arrays;
import java.util.HashSet;
import java.util.Iterator;
import java.util.Locale;
import java.util.Set;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/** Local user edits, independent of source identities and encrypted sessions. */
public final class LocalCourseStore {
    private final LocalDb db;
    private static final String[] FIELDS = {"name","teacher","location","color","day_of_week","start_time","end_time","week_pattern","semester_start_date","semester"};
    public LocalCourseStore(LocalDb db) { this.db = db; }

    public JSONObject save(String ws, long expected, long id, String syncId, JSONObject input) throws HenuError {
        JSONObject course = validate(input);
        if (id < 0 || expected < 0 || expected > LocalSettingsStore.MAX_SAFE_INTEGER || syncId == null
                || !syncId.matches("[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}")) throw HenuError.invalid("课程标识无效");
        if (id == 0) syncId = syncId.toLowerCase(Locale.ROOT);
        SQLiteDatabase writer = db.getWritableDatabase(); writer.beginTransaction();
        try {
            long current = db.workspaceRevision(ws);
            // A lost create reply can be retried with the same stable UUID.
            if (id == 0) try (Cursor c = writer.query("local_courses", null, "workspace_id=? AND sync_id=?", new String[]{ws,syncId},null,null,null)) {
                if (c.moveToFirst()) {
                    if (c.getInt(c.getColumnIndexOrThrow("active")) != 1 || !same(c,course)) throw new HenuError("REVISION_CONFLICT","此课程已发生变化");
                    JSONObject out = new JSONObject().put("revision",current).put("course_id",c.getLong(c.getColumnIndexOrThrow("id")));
                    writer.setTransactionSuccessful(); return out;
                }
            }
            if (current != expected) throw new HenuError("REVISION_CONFLICT", "本机课表已变化，请重新打开编辑");
            ContentValues values = new ContentValues();
            for (String key : FIELDS) if ("day_of_week".equals(key)) values.put(key,course.getInt(key)); else values.put(key,course.getString(key));
            String now = Projection.iso(System.currentTimeMillis());
            boolean created = id == 0;
            if (!created) {
                try (Cursor c = writer.query("local_courses",null,"workspace_id=? AND id=? AND active=1",new String[]{ws,String.valueOf(id)},null,null,null)) {
                    if (!c.moveToFirst() || !syncId.equals(c.getString(c.getColumnIndexOrThrow("sync_id")))) throw HenuError.notFound("课程不存在");
                    if (!course.getString("semester").equals(c.getString(c.getColumnIndexOrThrow("semester")))) throw HenuError.invalid("不能在编辑中移动学期");
                    if (same(c,course)) { writer.setTransactionSuccessful(); return new JSONObject().put("revision",current).put("course_id",id); }
                    JSONObject personal = overrides(writer,ws,id);
                    for (String key : FIELDS) {
                        String old = c.getString(c.getColumnIndexOrThrow(key));
                        if (!old.equals(String.valueOf(course.get(key)))) personal.put(key,course.get(key));
                    }
                    ContentValues v = new ContentValues(); v.put("workspace_id",ws); v.put("course_id",id); v.put("fields_json",personal.toString());
                    writer.insertWithOnConflict("local_course_overrides",null,v,SQLiteDatabase.CONFLICT_REPLACE);
                }
                values.put("updated_at",now);
                if (writer.update("local_courses",values,"workspace_id=? AND id=? AND active=1",new String[]{ws,String.valueOf(id)}) != 1) throw HenuError.notFound("课程不存在");
            } else {
                try (Cursor c = writer.rawQuery("SELECT next_course_id FROM local_workspaces WHERE id=?",new String[]{ws})) {
                    if (!c.moveToFirst()) throw HenuError.notFound("工作区不存在"); id = c.getLong(0);
                }
                if (id < 1 || id >= LocalSettingsStore.MAX_SAFE_INTEGER) throw HenuError.storage("课程编号超出范围");
                values.put("workspace_id",ws); values.put("id",id); values.put("sync_id",syncId);
                values.put("source_kind","MANUAL"); values.put("active",1); values.put("created_at",now); values.put("updated_at",now);
                writer.insertOrThrow("local_courses",null,values);
            }
            if (current >= LocalSettingsStore.MAX_SAFE_INTEGER) throw HenuError.storage("数据版本超出范围");
            ContentValues bump = new ContentValues(); bump.put("revision",current+1); if (created) bump.put("next_course_id",id+1);
            if (writer.update("local_workspaces",bump,"id=?",new String[]{ws}) != 1) throw HenuError.notFound("工作区不存在");
            JSONObject out = new JSONObject().put("revision",current+1).put("course_id",id);
            writer.setTransactionSuccessful(); return out;
        } catch (HenuError e) { throw e; }
        catch (Exception e) { throw HenuError.storage("课程未能保存"); }
        finally { writer.endTransaction(); }
    }

    public JSONObject delete(String ws, long expected, long id) throws HenuError {
        if (id <= 0 || expected < 0 || expected >= LocalSettingsStore.MAX_SAFE_INTEGER) throw HenuError.invalid("课程标识无效");
        SQLiteDatabase writer = db.getWritableDatabase(); writer.beginTransaction();
        try {
            long current = db.workspaceRevision(ws);
            if (current != expected) throw new HenuError("REVISION_CONFLICT","本机课表已变化");
            ContentValues values = new ContentValues(); values.put("active",0); values.put("updated_at",Projection.iso(System.currentTimeMillis()));
            if (writer.update("local_courses",values,"workspace_id=? AND id=? AND active=1",new String[]{ws,String.valueOf(id)}) != 1) throw HenuError.notFound("课程不存在");
            ContentValues bump = new ContentValues(); bump.put("revision",current+1);
            if (writer.update("local_workspaces",bump,"id=?",new String[]{ws}) != 1) throw HenuError.notFound("工作区不存在");
            JSONObject result = new JSONObject().put("revision",current+1); writer.setTransactionSuccessful(); return result;
        } catch (HenuError e) { throw e; } catch (Exception e) { throw HenuError.storage("课程未能删除"); }
        finally { writer.endTransaction(); }
    }

    static JSONObject overrides(SQLiteDatabase db, String ws, long id) throws HenuError {
        try (Cursor c = db.query("local_course_overrides",new String[]{"fields_json"},"workspace_id=? AND course_id=?",new String[]{ws,String.valueOf(id)},null,null,null)) {
            return c.moveToFirst() ? new JSONObject(c.getString(0)) : new JSONObject();
        } catch (Exception e) { throw HenuError.storage("个人课程修改读取失败"); }
    }
    private static boolean same(Cursor c, JSONObject course) throws Exception {
        for (String key : FIELDS) if (!c.getString(c.getColumnIndexOrThrow(key)).equals(String.valueOf(course.get(key)))) return false;
        return true;
    }
    private static JSONObject validate(JSONObject in) throws HenuError {
        if (in == null || in.length() != FIELDS.length) throw HenuError.invalid("课程字段不完整");
        Set<String> allowed = new HashSet<>(Arrays.asList(FIELDS));
        Iterator<String> keys = in.keys(); while (keys.hasNext()) if (!allowed.contains(keys.next())) throw HenuError.invalid("未知课程字段");
        JSONObject out = new JSONObject();
        try {
            for (String key : FIELDS) {
                Object raw = in.get(key);
                if ("day_of_week".equals(key)) {
                    if (!(raw instanceof Number) || ((Number)raw).doubleValue() != ((Number)raw).intValue() || ((Number)raw).intValue()<1 || ((Number)raw).intValue()>7) throw HenuError.invalid("星期无效");
                    out.put(key,((Number)raw).intValue()); continue;
                }
                if (!(raw instanceof String)) throw HenuError.invalid("课程字段类型无效");
                String value = ((String)raw).trim(); int max = ("teacher".equals(key)||"location".equals(key)) ? 512 : "week_pattern".equals(key) ? 1024 : 256;
                if (value.codePointCount(0,value.length())>max || value.indexOf('\0')>=0) throw HenuError.invalid("课程字段过长");
                out.put(key,value);
            }
            if (out.getString("name").isEmpty() || !out.getString("semester").matches("[0-9]{4}S[12]")) throw HenuError.invalid("课程名称或学期无效");
            for (String f : new String[]{"start_time","end_time"}) if (!out.getString(f).matches("(?:[01][0-9]|2[0-3]):[0-5][0-9]")) throw HenuError.invalid("课程时间无效");
            if (out.getString("start_time").compareTo(out.getString("end_time"))>=0) throw HenuError.invalid("结束时间需晚于开始时间");
            LocalDate date = LocalDate.parse(out.getString("semester_start_date"));
            if (!date.toString().equals(out.getString("semester_start_date"))) throw HenuError.invalid("日期无效");
            String color = out.getString("color").toUpperCase(Locale.ROOT);
            if (!color.matches("#[0-9A-F]{6}")) throw HenuError.invalid("颜色无效"); out.put("color",color);
            String weeks = out.getString("week_pattern").replaceAll("\\s","");
            if (weeks.isEmpty()) weeks = "1-18周";
            Pattern rule = Pattern.compile("^第?([1-9][0-9]?)(?:-([1-9][0-9]?))?(?:周(?:[（(]?[单双全]周?[)）]?)?)?$");
            for (String segment : weeks.split("[,，;；、/]",-1)) {
                Matcher m = rule.matcher(segment);
                if (!m.matches()) throw HenuError.invalid("周次规则无效");
                int first = Integer.parseInt(m.group(1)), last = m.group(2)==null ? first : Integer.parseInt(m.group(2));
                if (first>last || last>18) throw HenuError.invalid("周次需在1至18周内");
            }
            if (Projection.expandWeekPattern(weeks,18).isEmpty()) throw HenuError.invalid("周次没有有效教学周");
            out.put("week_pattern",weeks); return out;
        } catch (HenuError e) { throw e; } catch (Exception e) { throw HenuError.invalid("课程内容无效"); }
    }
}
