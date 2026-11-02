package com.xique.assistant.henu.parse;

import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;

import com.xique.assistant.henu.ops.HenuService;
import com.xique.assistant.henu.storage.LocalDb;
import com.xique.assistant.henu.storage.LocalSettingsStore;
import com.xique.assistant.henu.storage.Previews;
import com.xique.assistant.henu.storage.Projection;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * Local-v2 preview builder. Fills times from the confirmed settings snapshot,
 * validates the semester label and computes all six diff arrays. Legacy rows
 * are only offered as candidates when the match is unique in both directions;
 * no successful commit is possible for changed/missing sources in M2.
 */
public final class PreviewBuilder {

    private PreviewBuilder() {
    }

    public static JSONObject build(LocalDb db, String previewId, String ws, String semester,
                                   JSONObject parsed, long baseRevision,
                                   long connectionGeneration, long expiresAt)
            throws org.json.JSONException, com.xique.assistant.henu.ops.HenuError {
        return build(db, previewId, ws, semester, parsed, baseRevision,
                connectionGeneration, expiresAt, null);
    }

    public static JSONObject build(LocalDb db, String previewId, String ws, String semester,
                                   JSONObject parsed, long baseRevision,
                                   long connectionGeneration, long expiresAt,
                                   JSONObject settings)
            throws org.json.JSONException, com.xique.assistant.henu.ops.HenuError {
        JSONArray inCourses = parsed.optJSONArray("courses");
        JSONArray inIssues = parsed.optJSONArray("issues");
        if (inCourses == null) inCourses = new JSONArray();
        if (inIssues == null) inIssues = new JSONArray();
        boolean configured = settings != null && LocalSettingsStore.fullyConfigured(settings);
        int totalWeeks = settings == null ? 18 : settings.optInt("total_weeks", 18);

        JSONArray courses = new JSONArray();
        JSONArray added = new JSONArray();
        JSONArray unchanged = new JSONArray();
        JSONArray changed = new JSONArray();
        JSONArray missing = new JSONArray();
        JSONArray legacyCandidates = new JSONArray();
        JSONArray manualKept = new JSONArray();
        Map<Integer, String> courseIds = new HashMap<>();
        List<JSONObject> previewSessions = new ArrayList<>();
        Set<String> previewHashes = new HashSet<>();
        Map<String,Integer> identityCounts = new HashMap<>();
        int sessionCounter = 0;

        for (int i = 0; i < inCourses.length(); i++) {
            JSONObject c = inCourses.getJSONObject(i);
            JSONObject course = new JSONObject();
            String previewCourseId = LocalDb.newUuid();
            courseIds.put(c.optInt("course_index", i), previewCourseId);
            course.put("preview_course_id", previewCourseId);
            course.put("name", c.optString("name"));
            course.put("upstream_course_code", c.isNull("upstream_course_code")
                    ? JSONObject.NULL : c.get("upstream_course_code"));
            course.put("teaching_class_code", c.isNull("teaching_class_code")
                    ? JSONObject.NULL : c.get("teaching_class_code"));
            course.put("credits", c.isNull("credits") ? JSONObject.NULL : c.get("credits"));
            course.put("teachers", c.optJSONArray("teachers") == null
                    ? new JSONArray() : c.getJSONArray("teachers"));
            JSONArray sessionsIn = c.optJSONArray("sessions");
            JSONArray sessions = new JSONArray();
            if (sessionsIn != null) {
                for (int j = 0; j < sessionsIn.length(); j++) {
                    JSONObject s = sessionsIn.getJSONObject(j);
                    JSONObject out = new JSONObject();
                    out.put("preview_session_id", LocalDb.newUuid());
                    out.put("weekday", s.optInt("weekday"));
                    out.put("sections", s.optJSONArray("sections") == null
                            ? new JSONArray() : s.getJSONArray("sections"));
                    out.put("weeks", s.optJSONArray("weeks") == null
                            ? new JSONArray() : s.getJSONArray("weeks"));
                    out.put("location", s.optString("location"));
                    out.put("teacher", s.optString("teacher"));
                    String[] times = configured
                            ? Projection.times(settings, out.getJSONArray("sections")) : null;
                    out.put("start_time", times == null ? JSONObject.NULL : times[0]);
                    out.put("end_time", times == null ? JSONObject.NULL : times[1]);
                    sessions.put(out);
                    sessionCounter++;
                    JSONObject holder = new JSONObject();
                    holder.put("course", course);
                    holder.put("session", out);
                    previewSessions.add(holder);
                    String identity = Projection.identity(semester, course, out);
                    previewHashes.add(identity);
                    identityCounts.put(identity, identityCounts.getOrDefault(identity,0)+1);
                }
            }
            course.put("sessions", sessions);
            courses.put(course);
        }

        Map<String,Integer> storedCounts = new HashMap<>();
        SQLiteDatabase reader = db.getReadableDatabase();
        try (Cursor c = reader.query("local_source_sessions",
                new String[]{"session_id", "course_id", "content_sha", "source_json"},
                "workspace_id=? AND active=1", new String[]{ws}, null, null, null)) {
            while (c.moveToNext()) {
                JSONObject source = new JSONObject(c.getString(3));
                String hash = Projection.storedIdentity(db, ws, semester, c.getString(0), source);
                if (hash == null) continue;
                storedCounts.put(hash, storedCounts.getOrDefault(hash,0)+1);
                if (previewHashes.contains(hash)) continue;
                JSONObject item = new JSONObject();
                item.put("session_id", c.getString(0));
                item.put("course_id", c.getString(1));
                try {
                    item.put("source", new JSONObject(c.getString(3)));
                } catch (org.json.JSONException ignored) {
                }
                missing.put(item);
            }
        }
        Set<String> unchangedHashes = storedCounts.keySet();

        Map<String, List<JSONObject>> legacyByKey = new LinkedHashMap<>();
        Map<Long, JSONObject> legacyRows = new LinkedHashMap<>();
        try (Cursor c = reader.query("local_courses",
                new String[]{"id", "name", "day_of_week", "start_time", "end_time",
                        "week_pattern", "location", "teacher", "color"},
                "workspace_id=? AND semester=? AND active=1 AND source_kind=?",
                new String[]{ws, semester, Projection.SOURCE_LEGACY}, null, null, "id")) {
            while (c.moveToNext()) {
                JSONObject row = new JSONObject();
                row.put("course_id", c.getLong(0));
                row.put("name", c.getString(1));
                row.put("weekday", c.getInt(2));
                row.put("start_time", c.getString(3));
                row.put("end_time", c.getString(4));
                row.put("week_pattern", c.getString(5));
                row.put("location", c.getString(6));
                row.put("teacher", c.getString(7));
                row.put("color", c.getString(8));
                String key = Projection.legacyMatchKey(c.getString(1), c.getInt(2),
                        c.getString(3), c.getString(4), c.getString(5), totalWeeks);
                if (!key.isEmpty()) {
                    legacyByKey.computeIfAbsent(key, k -> new ArrayList<>()).add(row);
                }
                legacyRows.put(c.getLong(0), row);
            }
        }
        Map<String, List<JSONObject>> previewByKey = new LinkedHashMap<>();
        for (JSONObject holder : previewSessions) {
            String key = configured
                    ? Projection.previewMatchKey(holder.getJSONObject("course"),
                    holder.getJSONObject("session"), totalWeeks) : "";
            if (!key.isEmpty()) {
                previewByKey.computeIfAbsent(key, k -> new ArrayList<>()).add(holder);
            }
        }
        Set<Long> candidateIds = new HashSet<>();
        Set<String> candidateSessionIds = new HashSet<>();
        for (Map.Entry<String, List<JSONObject>> entry : previewByKey.entrySet()) {
            List<JSONObject> previews = entry.getValue();
            List<JSONObject> legacies = legacyByKey.get(entry.getKey());
            if (previews.size() != 1 || legacies == null || legacies.size() != 1) continue;
            JSONObject session = previews.get(0).getJSONObject("session");
            String sid = session.getString("preview_session_id");
            JSONObject legacy = legacies.get(0);
            JSONObject candidate = new JSONObject();
            candidate.put("preview_session_id", sid);
            candidate.put("course_id", legacy.getLong("course_id"));
            candidate.put("name", legacy.getString("name"));
            candidate.put("weekday", legacy.getInt("weekday"));
            candidate.put("start_time", legacy.getString("start_time"));
            candidate.put("end_time", legacy.getString("end_time"));
            legacyCandidates.put(candidate);
            candidateIds.add(legacy.getLong("course_id"));
            candidateSessionIds.add(sid);
        }
        for (Long id : legacyRows.keySet()) {
            if (candidateIds.contains(id)) continue;
            manualKept.put(new JSONObject(legacyRows.get(id).toString()));
        }
        for (JSONObject holder : previewSessions) {
            String hash = Projection.identity(semester, holder.getJSONObject("course"), holder.getJSONObject("session"));
            String sid = holder.getJSONObject("session").getString("preview_session_id");
            if (unchangedHashes.contains(hash) && storedCounts.get(hash) == 1 && identityCounts.get(hash) == 1) {
                unchanged.put(sid);
            } else if (!candidateSessionIds.contains(sid)) {
                added.put(sid);
            }
        }

        JSONArray issues = new JSONArray();
        boolean hasLocationIssue = false;
        for (int i = 0; i < inIssues.length(); i++) {
            JSONObject src = inIssues.getJSONObject(i);
            JSONObject issue = new JSONObject();
            issue.put("code", src.optString("code"));
            issue.put("severity", src.optString("severity"));
            issue.put("message", src.optString("message"));
            issue.put("preview_course_id", src.isNull("course_index")
                    ? JSONObject.NULL : courseIds.get(src.optInt("course_index")));
            issue.put("preview_session_id", JSONObject.NULL);
            issues.put(issue);
            if ("MISSING_LOCATION".equals(src.optString("code"))) hasLocationIssue = true;
        }
        if (!configured) {
            issues.put(new JSONObject().put("code", "TIMES_REQUIRED")
                    .put("severity", "BLOCKING").put("message", "学期作息尚未在本机确认，暂不能生成起止时刻"));
            issues.put(new JSONObject().put("code", "WEEK_ANCHOR_REQUIRED")
                    .put("severity", "BLOCKING").put("message", "教学周锚点尚未确认"));
        }
        for (JSONObject holder : previewSessions) {
            if (!hasLocationIssue && holder.getJSONObject("session").optString("location").isEmpty()) {
                issues.put(new JSONObject().put("code", "MISSING_LOCATION")
                        .put("severity", "WARNING").put("message", "这条安排没有教室"));
            }
        }
        if (legacyCandidates.length() > 0) {
            issues.put(new JSONObject().put("code", "LEGACY_CONTENT_PRESERVED")
                    .put("severity", "WARNING")
                    .put("message", "将沿用匹配的本机内容并保留个人修改"));
        }
        boolean ambiguous = identityCounts.values().stream().anyMatch(n -> n > 1)
            || storedCounts.values().stream().anyMatch(n -> n > 1);
        if (ambiguous) issues.put(new JSONObject().put("code","AMBIGUOUS_SOURCE_SESSION").put("severity","BLOCKING")
            .put("message","来源安排无法唯一匹配，请保留原课表并进一步核对"));
        boolean mapping = configured && settings.optString("semester_label").equals(parsed.optString("semester_label"));
        if (configured && !mapping) issues.put(new JSONObject().put("code","SEMESTER_MAPPING_REQUIRED")
            .put("severity","BLOCKING").put("message","学校学期与本机设置不一致，请核对"));
        boolean parseBlocking = false;
        for (int i=0;i<inIssues.length();i++) if ("BLOCKING".equals(inIssues.getJSONObject(i).optString("severity"))) parseBlocking=true;

        JSONObject stats = parsed.optJSONObject("stats");
        int unparsed = stats == null ? 0 : stats.optInt("unparsed_fragments", 0);
        String status;
        if (courses.length() == 0 || sessionCounter == 0 || unparsed > 0 || parseBlocking) {
            status = "INVALID";
        } else if (!configured || !mapping) {
            status = "NEEDS_CONFIGURATION";
        } else if (missing.length() > 0 || changed.length() > 0 || ambiguous) {
            status = "NEEDS_REVIEW";
        } else {
            status = "READY";
        }

        JSONObject diff = new JSONObject();
        diff.put("added", added);
        diff.put("unchanged", unchanged);
        diff.put("changed", changed);
        diff.put("missing", missing);
        diff.put("legacy_link_candidates", legacyCandidates);
        diff.put("manual_kept", manualKept);

        JSONObject preview = new JSONObject();
        preview.put("schema_version", 1);
        preview.put("preview_id", previewId);
        preview.put("semester", semester);
        preview.put("semester_label", parsed.optString("semester_label"));
        preview.put("status", status);
        preview.put("base_revision", baseRevision);
        preview.put("connection_generation", connectionGeneration);
        preview.put("expires_at", HenuService.iso(expiresAt));
        preview.put("stats", stats == null ? new JSONObject() : stats);
        preview.put("courses", courses);
        preview.put("issues", issues);
        preview.put("diff", diff);
        preview.put("can_commit", "READY".equals(status));

        Previews.insert(db, previewId, ws, semester, status,
                baseRevision, connectionGeneration, expiresAt, preview.toString());
        return preview;
    }
}
