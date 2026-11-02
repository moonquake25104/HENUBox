package com.xique.assistant.henu.storage;

import android.content.ContentValues;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;

import com.xique.assistant.henu.ops.HenuError;
import com.xique.assistant.henu.ops.HenuService;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.TreeSet;

/** First-save commit: sources, projections, receipt and revision in one transaction. */
public final class CommitService {

    private CommitService() {
    }

    public static JSONObject commit(LocalDb db, String ws, String previewId,
                                    long expectedRevision, String idempotencyKey,
                                    JSONArray acknowledged) throws HenuError {
        if (acknowledged == null || expectedRevision < 0 || expectedRevision > 9007199254740991L)
            throw HenuError.invalid("提交参数无效");
        SQLiteDatabase writer = db.getWritableDatabase();
        writer.beginTransaction();
        try {
            JSONObject receipt = commitLocked(db, ws, previewId, expectedRevision, idempotencyKey, acknowledged);
            writer.setTransactionSuccessful(); return receipt;
        } catch (RuntimeException e) { throw HenuError.storage("课表保存失败，原数据已保留"); }
        finally { writer.endTransaction(); }
    }

    private static JSONObject commitLocked(LocalDb db, String ws, String previewId,
                                    long expectedRevision, String idempotencyKey,
                                    JSONArray acknowledged) throws HenuError {
        String key = idempotencyKey == null ? "" : idempotencyKey.trim().toLowerCase();
        if (!key.matches("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")) {
            throw HenuError.invalid("幂等键必须是 UUID");
        }
        List<String> ack = normalized(acknowledged);
        String digest = Projection.sha256(ws + "\n" + previewId + "\n" + expectedRevision + "\n"
                + String.join(",", ack));
        JSONObject replay = Receipts.replay(db, ws, key, digest);
        if (replay != null) return replay;

        Previews.Row row = Previews.read(db, ws, previewId);
        if (row == null) throw HenuError.notFound("预览不存在");
        long now = System.currentTimeMillis();
        if (row.committedAt != null) {
            Receipts.Stored original = Receipts.byCommit(db, ws, row.commitId);
            if (original == null) throw HenuError.storage("回执不存在");
            if (!digest.equals(original.requestSha256)) throw new HenuError("IDEMPOTENCY_CONFLICT", "提交内容与原回执不一致");
            Receipts.addAlias(db, ws, key, row.commitId, now);
            return original.receipt;
        }
        if (row.expiresAt <= now) throw new HenuError("PREVIEW_EXPIRED", "预览已过期");
        if (db.workspaceRevision(ws) != expectedRevision || row.baseRevision != expectedRevision) {
            throw new HenuError("REVISION_CONFLICT", "本机数据已变化");
        }
        ConnectionRow connection = ConnectionRow.read(db, ws);
        if ((connection == null ? 0 : connection.generation) != row.connectionGeneration) {
            throw new HenuError("UPSTREAM_ACCOUNT_CHANGED", "学校连接已变化");
        }
        JSONObject payload = parse(row.payload);
        List<String> warnings = warningCodes(payload.optJSONArray("issues"));
        if (!new TreeSet<>(ack).equals(new TreeSet<>(warnings))) {
            throw new HenuError("WARNING_ACK_REQUIRED", "警告确认不完整");
        }
        if (!"READY".equals(row.status) || !payload.optBoolean("can_commit", false)) {
            throw new HenuError("PREVIEW_NOT_READY", "预览尚未就绪");
        }

        JSONObject settings = LocalSettingsStore.readStored(db, ws, row.semester);
        if (settings == null || !LocalSettingsStore.fullyConfigured(settings)) {
            throw new HenuError("PREVIEW_NOT_READY", "作息不完整");
        }
        JSONObject diff;
        try {
            diff = payload.getJSONObject("diff");
        } catch (org.json.JSONException e) {
            throw HenuError.storage("预览 diff 损坏");
        }
        boolean noChanges = diff.optJSONArray("added").length() == 0
                && diff.optJSONArray("changed").length() == 0
                && diff.optJSONArray("missing").length() == 0
                && diff.optJSONArray("legacy_link_candidates").length() == 0;
        SQLiteDatabase writer = db.getWritableDatabase();
        if (noChanges) {
            writer.beginTransaction();
            try {
                if (db.workspaceRevision(ws) != expectedRevision) {
                    throw new HenuError("REVISION_CONFLICT", "本机数据已变化");
                }
                String commitId = LocalDb.newUuid();
                JSONObject receipt = new JSONObject();
                receipt.put("commit_id", commitId);
                receipt.put("preview_id", previewId);
                receipt.put("semester", row.semester);
                receipt.put("revision", expectedRevision);
                receipt.put("no_changes", true);
                receipt.put("courses_created", 0);
                receipt.put("sessions_created", 0);
                receipt.put("legacy_linked", 0);
                receipt.put("manual_kept", diff.optJSONArray("manual_kept").length());
                receipt.put("projection_created", 0);
                receipt.put("projection_updated", 0);
                receipt.put("unchanged", diff.optJSONArray("unchanged").length());
                receipt.put("warning_codes", new JSONArray(ack));
                receipt.put("committed_at", Projection.iso(now));
                Receipts.insert(db, ws, commitId, key, digest, receipt, now,
                        now + Receipts.RETENTION_MS);
                JSONObject committed = new JSONObject(payload.toString());
                committed.put("status", "COMMITTED");
                committed.put("commit_id", commitId);
                Previews.markCommitted(db, ws, previewId, commitId,
                        committed.toString(), now);
                writer.setTransactionSuccessful();
                return receipt;
            } catch (HenuError e) {
                throw e;
            } catch (org.json.JSONException e) {
                throw HenuError.storage("回执序列化失败");
            } finally {
                writer.endTransaction();
            }
        }
        writer.beginTransaction();
        try {
            // Re-read under the lock; a concurrent settings/connection change must roll back.
            if (db.workspaceRevision(ws) != expectedRevision) {
                throw new HenuError("REVISION_CONFLICT", "本机数据已变化");
            }
            ConnectionRow locked = ConnectionRow.read(db, ws);
            if ((locked == null ? 0 : locked.generation) != row.connectionGeneration) {
                throw new HenuError("UPSTREAM_ACCOUNT_CHANGED", "学校连接已变化");
            }
            Previews.Row lockedPreview = Previews.read(db, ws, previewId);
            if (lockedPreview == null || lockedPreview.committedAt != null) {
                throw new HenuError("PREVIEW_NOT_READY", "预览状态已变化");
            }
            if (lockedPreview.expiresAt <= now) throw new HenuError("PREVIEW_EXPIRED", "预览已过期");

            String anchor = Projection.anchor(settings);
            if (anchor.isEmpty()) throw new HenuError("PREVIEW_NOT_READY", "教学周锚点未确认");
            JSONArray courses = payload.getJSONArray("courses");
            JSONArray candidates = payload.getJSONObject("diff")
                    .optJSONArray("legacy_link_candidates");
            Map<String, JSONObject> candidateBySession = new LinkedHashMap<>();
            if (candidates != null) {
                for (int i = 0; i < candidates.length(); i++) {
                    JSONObject candidate = candidates.getJSONObject(i);
                    candidateBySession.put(candidate.getString("preview_session_id"), candidate);
                }
            }

            long nextId = nextCourseId(writer, ws);
            Set<String> unchangedIds = new HashSet<>();
            JSONArray same = diff.getJSONArray("unchanged");
            for (int i = 0; i < same.length(); i++) unchangedIds.add(same.getString(i));
            int sessionsCreated = 0, legacyLinked = 0, projectionCreated = 0;
            Set<String> existingCourses = new HashSet<>(), createdCourses = new HashSet<>();
            try (Cursor old = writer.query("local_source_sessions",new String[]{"session_id","source_json"},"workspace_id=? AND active=1",new String[]{ws},null,null,null)) {
                while (old.moveToNext()) {
                    JSONObject stored = new JSONObject(old.getString(1));
                    if (Projection.storedIdentity(db,ws,row.semester,old.getString(0),stored) != null)
                        existingCourses.add(Projection.courseKeyFromSource(stored));
                }
            }
            Set<String> linkedCourseIds = new HashSet<>();
            String nowIso = Projection.iso(now);
            for (int i = 0; i < courses.length(); i++) {
                JSONObject course = courses.getJSONObject(i);
                JSONArray sessions = course.optJSONArray("sessions");
                if (sessions == null) continue;
                for (int j = 0; j < sessions.length(); j++) {
                    JSONObject session = sessions.getJSONObject(j);
                    String previewSessionId = session.getString("preview_session_id");
                    if (unchangedIds.contains(previewSessionId)) continue;
                    String logicalKey = Projection.courseKey(course);
                    if (!existingCourses.contains(logicalKey)) createdCourses.add(logicalKey);
                    String sourceSessionId = LocalDb.newUuid();
                    JSONObject source = new JSONObject();
                    source.put("session_id", sourceSessionId);
                    source.put("semester", row.semester);
                    source.put("course_name", course.optString("name"));
                    source.put("upstream_course_code", course.opt("upstream_course_code"));
                    source.put("teaching_class_code", course.opt("teaching_class_code"));
                    source.put("credits", course.opt("credits"));
                    source.put("weekday", session.optInt("weekday"));
                    source.put("sections", session.optJSONArray("sections"));
                    source.put("weeks", session.optJSONArray("weeks"));
                    source.put("location", session.optString("location"));
                    source.put("teacher", Projection.teacher(course, session));
                    String entryBase = sourceSessionId;
                    String hash = Projection.contentHash(course, session);
                    ContentValues src = new ContentValues();
                    src.put("workspace_id", ws);
                    src.put("session_id", sourceSessionId);
                    src.put("course_id", Projection.courseKey(course));
                    src.put("entry_id", entryBase);
                    src.put("content_sha", hash);
                    src.put("source_json", source.toString());
                    src.put("active", 1);
                    src.put("created_at", nowIso);
                    writer.insertOrThrow("local_source_sessions", null, src);
                    sessionsCreated++;

                    JSONObject candidate = candidateBySession.get(previewSessionId);
                    List<Projection.Segment> segments = Projection.segments(
                            session.getJSONArray("sections"));
                    String weekPattern = Projection.weekPattern(session.getJSONArray("weeks"));
                    for (int ordinal = 0; ordinal < segments.size(); ordinal++) {
                        String entryId = Projection.entryKey(sourceSessionId, ordinal);
                        String[] times = Projection.times(settings,
                                sectionsOf(segments.get(ordinal)));
                        if (times == null) throw HenuError.invalid("作息缺少所需节次");
                        if (candidate != null && ordinal == 0) {
                            long legacyId = candidate.getLong("course_id");
                            // Preserve explicit personal values separately from
                            // the school source, as well as the original row.
                            JSONObject personal = new JSONObject();
                            try (Cursor old = writer.query("local_courses", new String[]{"name","teacher","location","color"},
                                    "workspace_id=? AND id=? AND source_kind=? AND active=1",
                                    new String[]{ws,String.valueOf(legacyId),Projection.SOURCE_LEGACY},null,null,null)) {
                                if (!old.moveToFirst()) throw HenuError.storage("本机关联目标已变化");
                                personal.put("name",old.getString(0)).put("teacher",old.getString(1))
                                    .put("location",old.getString(2)).put("color",old.getString(3));
                            }
                            source.put("personal_overrides",personal);
                            ContentValues savedSource = new ContentValues(); savedSource.put("source_json",source.toString());
                            writer.update("local_source_sessions",savedSource,"workspace_id=? AND session_id=?",new String[]{ws,sourceSessionId});
                            ContentValues v = new ContentValues();
                            v.put("source_kind", Projection.SOURCE_HENU);
                            v.put("source_entry_id", entryId);
                            v.put("source_course_id", Projection.courseKey(course));
                            v.put("updated_at", nowIso);
                            if (writer.update("local_courses", v,
                                    "workspace_id=? AND id=?", new String[]{ws,
                                            String.valueOf(legacyId)}) != 1) throw HenuError.storage("本机关联失败");
                            linkedCourseIds.add(String.valueOf(legacyId));
                            legacyLinked++;
                            continue;
                        }
                        ContentValues v = new ContentValues();
                        long id = nextId++;
                        if (id > 9007199254740991L) throw HenuError.storage("课程编号超出范围");
                        v.put("workspace_id", ws);
                        v.put("id", id);
                        v.put("semester", row.semester);
                        v.put("sync_id", Projection.stableSyncId(ws, sourceSessionId, ordinal));
                        v.put("name", course.optString("name"));
                        v.put("day_of_week", session.optInt("weekday"));
                        v.put("start_time", times[0]);
                        v.put("end_time", times[1]);
                        v.put("week_pattern", weekPattern);
                        v.put("semester_start_date", anchor);
                        v.put("location", session.optString("location"));
                        v.put("teacher", Projection.teacher(course, session));
                        v.put("color", Projection.DEFAULT_COLOR);
                        v.put("source_kind", Projection.SOURCE_HENU);
                        v.put("source_entry_id", entryId);
                        v.put("source_course_id", Projection.courseKey(course));
                        v.put("active", 1);
                        v.put("created_at", nowIso);
                        v.put("updated_at", nowIso);
                        writer.insertOrThrow("local_courses", null, v);
                        projectionCreated++;
                    }
                }
            }
            ContentValues bump = new ContentValues();
            bump.put("next_course_id", nextId);
            bump.put("revision", expectedRevision + 1);
            if (writer.update("local_workspaces", bump, "id=?", new String[]{ws}) != 1) {
                throw HenuError.notFound("工作区不存在");
            }
            String commitId = LocalDb.newUuid();
            JSONObject receipt = new JSONObject();
            receipt.put("commit_id", commitId);
            receipt.put("preview_id", previewId);
            receipt.put("semester", row.semester);
            receipt.put("revision", expectedRevision + 1);
            receipt.put("no_changes", false);
            receipt.put("courses_created", createdCourses.size());
            receipt.put("sessions_created", sessionsCreated);
            receipt.put("legacy_linked", legacyLinked);
            receipt.put("manual_kept", payload.getJSONObject("diff")
                    .optJSONArray("manual_kept") == null ? 0
                    : payload.getJSONObject("diff").getJSONArray("manual_kept").length());
            receipt.put("projection_created", projectionCreated);
            receipt.put("projection_updated", 0);
            receipt.put("unchanged", payload.getJSONObject("diff")
                    .optJSONArray("unchanged") == null ? 0
                    : payload.getJSONObject("diff").getJSONArray("unchanged").length());
            receipt.put("warning_codes", new JSONArray(ack));
            receipt.put("committed_at", nowIso);
            Receipts.insert(db, ws, commitId, key, digest, receipt, now,
                    now + Receipts.RETENTION_MS);
            JSONObject committed = new JSONObject(payload.toString());
            committed.put("status", "COMMITTED");
            committed.put("commit_id", commitId);
            Previews.markCommitted(db, ws, previewId, commitId, committed.toString(), now);
            writer.setTransactionSuccessful();
            return receipt;
        } catch (HenuError e) {
            throw e;
        } catch (org.json.JSONException e) {
            throw HenuError.storage("回执序列化失败");
        } finally {
            writer.endTransaction();
        }
    }

    private static String sourceSessionId(SQLiteDatabase db, String ws, JSONObject session) {
        try (Cursor c = db.query("local_source_sessions", new String[]{"session_id"},
                "workspace_id=? AND content_sha=?", new String[]{ws,
                        Projection.sessionHash(session)}, null, null, null)) {
            if (c.moveToFirst()) return c.getString(0);
        } catch (HenuError ignored) {
        }
        return LocalDb.newUuid();
    }

    private static long nextCourseId(SQLiteDatabase db, String ws) throws HenuError {
        try (Cursor c = db.query("local_workspaces", new String[]{"next_course_id"},
                "id=?", new String[]{ws}, null, null, null)) {
            if (!c.moveToFirst()) throw HenuError.notFound("工作区不存在");
            return c.getLong(0);
        }
    }

    private static JSONArray sectionsOf(Projection.Segment segment) {
        JSONArray out = new JSONArray();
        for (int i = segment.first; i <= segment.last; i++) out.put(i);
        return out;
    }

    private static List<String> warningCodes(JSONArray issues) {
        Set<String> out = new TreeSet<>();
        if (issues != null) {
            for (int i = 0; i < issues.length(); i++) {
                JSONObject issue = issues.optJSONObject(i);
                if (issue != null && "WARNING".equals(issue.optString("severity"))) {
                    out.add(issue.optString("code"));
                }
            }
        }
        return new ArrayList<>(out);
    }

    private static List<String> normalized(JSONArray values) throws HenuError {
        Set<String> out = new TreeSet<>();
        if (values != null) {
            for (int i = 0; i < values.length(); i++) {
                Object value = values.opt(i);
                if (!(value instanceof String)) throw HenuError.invalid("警告码必须是字符串");
                String code = ((String) value).trim();
                if (code.isEmpty() || code.length() > 64) throw HenuError.invalid("警告码无效");
                out.add(code);
            }
        }
        return new ArrayList<>(out);
    }

    private static JSONObject parse(String payload) throws HenuError {
        try {
            return new JSONObject(payload == null ? "{}" : payload);
        } catch (org.json.JSONException e) {
            throw HenuError.storage("预览损坏");
        }
    }
}
