package com.xique.assistant.henu.ops;

import android.app.Activity;
import android.content.ComponentName;
import android.content.ContentValues;
import android.content.Context;
import android.database.sqlite.SQLiteDatabase;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.Message;
import android.os.Messenger;

import com.xique.assistant.henu.auth.AuthIpc;
import com.xique.assistant.henu.auth.HenuAuthActivity;
import com.xique.assistant.henu.crypto.SessionCrypto;
import com.xique.assistant.henu.http.SchoolClient;
import com.xique.assistant.henu.parse.ParseBridge;
import com.xique.assistant.henu.storage.CommitService;
import com.xique.assistant.henu.storage.ConnectionRow;
import com.xique.assistant.henu.storage.LegacyImport;
import com.xique.assistant.henu.storage.LocalDb;
import com.xique.assistant.henu.storage.LocalSettingsStore;
import com.xique.assistant.henu.storage.Operations;
import com.xique.assistant.henu.storage.Previews;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.File;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.Map;
import java.util.HashSet;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * M1 plain-Java service facade over the shared native core. All public
 * methods follow the §6 DTO shapes and throw {@link HenuError} with fixed
 * safe codes. One worker thread; at most one active operation per workspace;
 * the auth process never writes the database.
 */
public final class HenuService {

    public static final long LOGIN_TTL_MS = 10 * 60 * 1000;
    public static final long PREVIEW_TTL_MS = 24 * 60 * 60 * 1000;
    public static final long OP_KEY_RETENTION_MS = 24 * 60 * 60 * 1000;

    private final Context context;
    private final LocalDb db;

    /** Shared storage handle for sibling services (classroom) in this process. */
    public LocalDb database() {
        return db;
    }

    interface SessionVerifier { VerifiedSession verify(Map<String, String> cookies) throws Exception; }
    static final class VerifiedSession {
        final SchoolClient.Term term;
        final Map<String, String> cookies;
        VerifiedSession(SchoolClient.Term term, Map<String, String> cookies) {
            this.term = term;
            this.cookies = new LinkedHashMap<>(cookies);
        }
    }
    private final SessionVerifier verifier;
    interface TimetableFetcher { Fetched fetch(Map<String, String> cookies) throws Exception; }
    static final class Fetched {
        final SchoolClient.Term term; final JSONObject parsed; final Map<String, String> cookies;
        Fetched(SchoolClient.Term term, JSONObject parsed, Map<String, String> cookies) {
            this.term = term; this.parsed = parsed; this.cookies = new LinkedHashMap<>(cookies);
        }
    }
    private final TimetableFetcher fetcher;
    private final Object loginLock = new Object();
    private final Set<String> scheduledVerifications = new HashSet<>();
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());

    /** Candidate cookies of the current login attempt, main-process memory only. */
    private final Map<String, String> loginCandidates = new LinkedHashMap<>();
    private String pendingLoginOpId;
    private String pendingLoginNonce;
    private long pendingConnectionGeneration;

    /** Receiver for frames from the auth process; registered per login attempt. */
    private Messenger authReceiver;

    public HenuService(Context context) {
        this(context, new LocalDb(context), cookies -> {
            SchoolClient client = new SchoolClient(cookies);
            SchoolClient.Term term = client.fetchTerm();
            return new VerifiedSession(term, client.currentCookies());
        });
    }

    HenuService(Context context, LocalDb database, SessionVerifier verifier) {
        this(context, database, verifier, cookies -> {
            SchoolClient client = new SchoolClient(cookies);
            SchoolClient.Term term = client.fetchTerm();
            return new Fetched(term, ParseBridge.parse(client.fetchTimetable(term)), client.currentCookies());
        });
    }

    HenuService(Context context, LocalDb database, SessionVerifier verifier, TimetableFetcher fetcher) {
        this.context = context.getApplicationContext();
        this.db = database;
        this.verifier = verifier;
        this.fetcher = fetcher;
        recoverInterrupted();
        Previews.sweep(db, System.currentTimeMillis());
    }

    private void recoverInterrupted() {
        android.database.sqlite.SQLiteDatabase w = db.getWritableDatabase();
        long now = System.currentTimeMillis();
        w.execSQL("UPDATE local_operations SET state='FAILED', error_code='OPERATION_INTERRUPTED',"
                + " error_message='进程重启，任务已中断', finished_at=?"
                + " WHERE state IN ('WAITING_FOR_USER','VERIFYING','QUEUED','RUNNING')", new Object[]{now});
    }

    // ------------------------------------------------------------------
    // XiqueLocal.bootstrap (M1 slice)
    // ------------------------------------------------------------------

    public JSONObject bootstrap() throws HenuError {
        android.database.sqlite.SQLiteDatabase writer = db.getWritableDatabase(); writer.beginTransaction();
        try { JSONObject result = bootstrapLocked(); writer.setTransactionSuccessful(); return result; }
        finally { writer.endTransaction(); }
    }

    private JSONObject bootstrapLocked() throws HenuError {
        Previews.sweep(db, System.currentTimeMillis());
        String ws = db.ensureWorkspace();
        long revision = db.workspaceRevision(ws);
        JSONObject out = new JSONObject();
        try {
            out.put("schema_version", 2);
            out.put("workspace_id", ws);
            out.put("revision", revision);
            out.put("mode", "LOCAL");
            out.put("saved_at", JSONObject.NULL);
            JSONArray courses = new JSONArray();
            String savedAt = null;
            try (android.database.Cursor c = db.getReadableDatabase().query("local_courses",
                    new String[]{"id", "sync_id", "name", "day_of_week", "start_time", "end_time",
                            "week_pattern", "semester_start_date", "location", "teacher", "color",
                            "semester", "created_at", "updated_at"},
                    "workspace_id=? AND active=1", new String[]{ws}, null, null, "id")) {
                while (c.moveToNext()) {
                    JSONObject course = new JSONObject();
                    course.put("id", c.getLong(0));
                    course.put("sync_id", c.getString(1));
                    course.put("name", c.getString(2));
                    course.put("day_of_week", c.getInt(3));
                    course.put("start_time", c.getString(4));
                    course.put("end_time", c.getString(5));
                    course.put("week_pattern", c.getString(6));
                    course.put("semester_start_date", c.getString(7));
                    course.put("location", c.getString(8));
                    course.put("teacher", c.getString(9));
                    course.put("color", c.getString(10));
                    course.put("semester", c.getString(11));
                    course.put("created_at", c.getString(12));
                    course.put("updated_at", c.getString(13));
                    course.put("deleted_at", JSONObject.NULL);
                    courses.put(course);
                    if (savedAt == null || c.getString(13).compareTo(savedAt) > 0) {
                        savedAt = c.getString(13);
                    }
                }
            }
            out.put("courses", courses);
            if (savedAt != null) out.put("saved_at", savedAt);
            int legacyCount = 0;
            try (android.database.Cursor c = db.getReadableDatabase().rawQuery(
                    "SELECT count(*) FROM local_courses WHERE workspace_id=? AND active=1"
                            + " AND source_kind=?", new String[]{ws, LegacyImport.SOURCE_KIND})) {
                if (c.moveToFirst()) legacyCount = c.getInt(0);
            }
            boolean migrated = LegacyImport.isComplete(db);
            JSONObject legacy = new JSONObject();
            legacy.put("state", legacyCount > 0 ? "PRESERVED" : (migrated ? "ADOPTED" : "NONE"));
            legacy.put("course_count", legacyCount);
            out.put("legacy", legacy);
            JSONObject caps = new JSONObject();
            caps.put("henu_direct", Build.VERSION.SDK_INT >= 28);
            caps.put("can_commit", true);
            caps.put("can_resolve", false);
            out.put("capabilities", caps);
        } catch (org.json.JSONException e) {
            throw HenuError.storage("序列化失败");
        }
        return out;
    }

    // ------------------------------------------------------------------
    // Connection
    // ------------------------------------------------------------------

    public JSONObject getConnection(String ws) throws HenuError {
        requireWorkspace(ws);
        ConnectionRow row = ConnectionRow.read(db, ws);
        Operations.ActiveOp active = Operations.ActiveOp.read(db, ws);
        JSONObject out = new JSONObject();
        try {
            out.put("connection_state", row == null ? "DISCONNECTED" : row.state);
            out.put("connection_generation", row == null ? 0 : row.generation);
            JSONObject caps = new JSONObject();
            caps.put("interactive_login", Build.VERSION.SDK_INT >= 28);
            caps.put("direct_fetch", true);
            caps.put("can_commit", true);
            caps.put("can_resolve", false);
            out.put("capabilities", caps);
            out.put("active_login_id", active != null && active.kind.equals("LOGIN") ? active.id : JSONObject.NULL);
            out.put("active_job_id", active != null && active.kind.equals("FETCH") ? active.id : JSONObject.NULL);
            if (row != null && row.termYear != null && row.termTerm != null && row.termLabel != null) {
                JSONObject term = new JSONObject();
                term.put("upstream_year", row.termYear);
                term.put("upstream_term", row.termTerm);
                term.put("label", row.termLabel);
                out.put("current_term", term);
            } else {
                out.put("current_term", JSONObject.NULL);
            }
            out.put("last_verified_at", row == null || row.verifiedAt == null ? JSONObject.NULL : iso(row.verifiedAt));
            out.put("last_checked_at", JSONObject.NULL); // M1: no check history table
            if (row != null && row.errorCode != null) {
                JSONObject err = new JSONObject();
                err.put("code", row.errorCode);
                err.put("message", row.errorMessage == null ? "" : row.errorMessage);
                err.put("occurred_at", iso(row.errorAt == null ? 0 : row.errorAt));
                out.put("last_error", err);
            } else {
                out.put("last_error", JSONObject.NULL);
            }
        } catch (org.json.JSONException e) {
            throw HenuError.storage("序列化失败");
        }
        return out;
    }

    // ------------------------------------------------------------------
    // Login
    // ------------------------------------------------------------------

    public JSONObject startLogin(String ws, String idempotencyKey, Activity launcher) throws HenuError {
        synchronized (loginLock) {
        requireWorkspace(ws);
        String key = normalizeKey(idempotencyKey);
        String digest = sha256("LOGIN\n" + ws);
        String existing = Operations.findIdempotent(db, ws, key, digest);
        if (existing != null) return getOperation(ws, existing);
        Operations.ActiveOp active = Operations.ActiveOp.read(db, ws);
        if (active != null) {
            throw new HenuError("OPERATION_IN_PROGRESS", "已有进行中的任务");
        }
        if (launcher == null) throw HenuError.invalid("缺少启动上下文");
        String opId = LocalDb.newUuid();
        long now = System.currentTimeMillis();
        Operations.insert(db, opId, ws, "LOGIN", "WAITING_FOR_USER", key, digest, now, now + LOGIN_TTL_MS);

        String nonce = UUID.randomUUID().toString().toLowerCase(Locale.ROOT);
        authReceiver = new Messenger(new Handler(Looper.getMainLooper()) {
            @Override
            public void handleMessage(Message msg) {
                handleAuthFrame(msg);
            }
        });
        pendingLoginOpId = opId;
        pendingLoginNonce = nonce;
        ConnectionRow current = ConnectionRow.read(db, ws);
        pendingConnectionGeneration = current == null ? 0 : current.generation;
        loginCandidates.clear();

        if (launcher == null) {
            throw HenuError.invalid("缺少启动上下文");
        }
        Intent intent = new Intent();
        intent.setComponent(new ComponentName(launcher, HenuAuthActivity.class));
        intent.putExtra(AuthIpc.EXTRA_OP_ID, opId);
        intent.putExtra(AuthIpc.EXTRA_NONCE, nonce);
        intent.putExtra(AuthIpc.EXTRA_MESSENGER, authReceiver);
        intent.putExtra(AuthIpc.EXTRA_DEADLINE, now + LOGIN_TTL_MS);
        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        try {
            launcher.startActivity(intent);
        } catch (Exception e) {
            Operations.finish(db, opId, "FAILED", "UNSUPPORTED_DEVICE", "无法打开学校登录窗口", null);
            throw new HenuError("UNSUPPORTED_DEVICE", "无法打开学校登录窗口");
        }
        return getOperation(ws, opId);
        }
    }

    private void handleAuthFrame(Message msg) {
        synchronized (loginLock) {
        if (authReceiver == null) return;
        Bundle data = msg.getData();
        if (data == null) return;
        // Same-UID enforcement: only our own process may deliver frames.
        if (msg.sendingUid != myUid()) return;
        String opId = data.getString(AuthIpc.KEY_OP_ID);
        String nonceEcho = data.getString(AuthIpc.KEY_NONCE);
        if (opId == null || !opId.equals(pendingLoginOpId) || !isFreshNonce(nonceEcho)) {
            return;
        }
        Operations.OpRow operation = Operations.get(db, db.ensureWorkspace(), opId);
        if (operation == null || isTerminal(operation.state)) return;
        if (System.currentTimeMillis() >= operation.expiresAt) {
            Operations.finish(db, opId, "EXPIRED", null, null, null);
            return;
        }
        switch (msg.what) {
            case AuthIpc.MSG_COOKIES: {
                String[] urls = data.getStringArray(AuthIpc.KEY_URLS);
                String[] values = data.getStringArray(AuthIpc.KEY_COOKIES);
                if (urls == null || values == null || urls.length != values.length) return;
                String[] allowed = {SchoolClient.ORIGIN + "/jw/common/showYearTerm.action",
                    SchoolClient.ORIGIN + "/student/xkjg.wdkb.jsp?menucode=S20301",
                    SchoolClient.ORIGIN + "/wsxk/xkjg.ckdgxsxdkchj_data10319.jsp"};
                if (!"WAITING_FOR_USER".equals(operation.state) || !java.util.Arrays.equals(urls, allowed)) return;
                Map<String, String> merged = new LinkedHashMap<>();
                int total = 0;
                for (String header : values) {
                    if (header == null || header.contains("\r") || header.contains("\n")) return;
                    int bytes = header.getBytes(StandardCharsets.UTF_8).length;
                    if (bytes > 16 * 1024) return;
                    total += bytes;
                    for (String pair : header.split(";")) {
                        int eq = pair.indexOf('=');
                        if (eq <= 0) continue;
                        String name = pair.substring(0, eq).trim();
                        String value = pair.substring(eq + 1).trim();
                        if (!name.isEmpty()) merged.put(name, value);
                    }
                }
                if (total > AuthIpc.MAX_FRAME_BYTES || merged.isEmpty()) return;
                loginCandidates.clear();
                loginCandidates.putAll(merged);
                Operations.transition(db, opId, "WAITING_FOR_USER", "VERIFYING", null, null);
                break;
            }
            case AuthIpc.MSG_CLOSED: {
                if (!isTerminal(operation.state)) {
                    Operations.finish(db, opId, "CANCELLED", null, null, null);
                    loginCandidates.clear(); pendingLoginOpId = null; pendingLoginNonce = null;
                }
                break;
            }
            case AuthIpc.MSG_ERROR: {
                Operations.transition(db, opId, "WAITING_FOR_USER", "FAILED",
                        "LOCAL_LOGIN_FAILED", "登录窗口异常关闭");
                break;
            }
            default:
                break;
        }
        }
    }

    private int myUid() {
        return context.getApplicationInfo().uid;
    }

    private boolean isFreshNonce(String echo) {
        return echo != null && pendingLoginNonce != null && MessageDigest.isEqual(
            pendingLoginNonce.getBytes(StandardCharsets.UTF_8), echo.getBytes(StandardCharsets.UTF_8));
    }

    /** Verifies the current candidate session against the school term API. */
    public JSONObject verifyLogin(String ws, String opId) throws HenuError {
        synchronized (loginLock) {
        requireWorkspace(ws);
        Operations.OpRow op = Operations.get(db, ws, opId);
        if (op == null) throw HenuError.notFound("任务不存在");
        if (!op.kind.equals("LOGIN")) throw HenuError.invalid("不是登录任务");
        if (!isTerminal(op.state) && System.currentTimeMillis() >= op.expiresAt) {
            Operations.finish(db, opId, "EXPIRED", null, null, null);
            return getOperation(ws, opId);
        }
        if (!op.state.equals("VERIFYING") && !op.state.equals("WAITING_FOR_USER")) {
            return operationView(op); // terminal: idempotent view
        }
        if (!opId.equals(pendingLoginOpId) || loginCandidates.isEmpty()) {
            return operationView(op); // nothing to verify yet; probe UI keeps polling
        }
        // Snapshot the candidate before handing it to a worker. Repeated button
        // taps must schedule exactly one request, not just keep returning VERIFYING.
        if (scheduledVerifications.add(opId)) {
            if ("WAITING_FOR_USER".equals(op.state)) Operations.transition(db, opId, op.state, "VERIFYING", null, null);
            final Map<String, String> candidate = new LinkedHashMap<>(loginCandidates);
            final long expectedGeneration = pendingConnectionGeneration;
            worker.submit(() -> runVerification(ws, opId, candidate, expectedGeneration));
        }
        return getOperation(ws, opId);
        }
    }

    private void runVerification(String ws, String opId, Map<String, String> candidate, long expectedGeneration) {
        try {
            VerifiedSession verified = verifier.verify(candidate);
            SchoolClient.Term term = verified.term;
            synchronized (loginLock) {
            android.database.sqlite.SQLiteDatabase writer = db.getWritableDatabase();
            writer.beginTransaction();
            try {
            if (!verificationCurrent(ws, opId, expectedGeneration)) { writer.setTransactionSuccessful(); return; }
            ConnectionRow existing = ConnectionRow.read(db, ws);
            String fingerprint = sha256(term.userCode);
            if (existing != null && existing.subjectFingerprint != null
                    && !existing.subjectFingerprint.equals(fingerprint)) {
                Operations.finish(db, opId, "FAILED", "UPSTREAM_ACCOUNT_CHANGED",
                        "学校账号与上次授权不一致", null);
                writer.setTransactionSuccessful();
                return;
            }
            long generation = (existing == null ? 0 : existing.generation) + 1;
            JSONObject payload = new JSONObject();
            payload.put("cookies", new JSONObject(verified.cookies));
            payload.put("user_code", term.userCode);
            byte[] sealed = SessionCrypto.seal(payload.toString().getBytes(StandardCharsets.UTF_8),
                    SessionCrypto.aad(ws, "henu", generation));
            ConnectionRow.upsertConnected(db, ws, generation, sealed, fingerprint,
                    term.year, term.term, term.label);
            Operations.finish(db, opId, "SUCCEEDED", null, null, null);
            writer.setTransactionSuccessful();
            } finally { writer.endTransaction(); }
            }
        } catch (SchoolClient.SchoolException e) {
            finishVerificationFailure(ws, opId, expectedGeneration, e.code, e.getMessage());
        } catch (Exception e) {
            finishVerificationFailure(ws, opId, expectedGeneration, "STORAGE_UNAVAILABLE", "会话保存失败");
        } finally {
            synchronized (loginLock) {
            if (opId.equals(pendingLoginOpId)) {
            loginCandidates.clear();
            pendingLoginOpId = null;
            pendingLoginNonce = null;
            }
            }
        }
    }

    private boolean verificationCurrent(String ws, String opId, long generation) {
        Operations.OpRow op = Operations.get(db, ws, opId);
        if (op == null || isTerminal(op.state)) return false;
        if (System.currentTimeMillis() >= op.expiresAt) {
            Operations.finish(db, opId, "EXPIRED", null, null, null);
            return false;
        }
        ConnectionRow row = ConnectionRow.read(db, ws);
        if ((row == null ? 0 : row.generation) != generation || !opId.equals(pendingLoginOpId)) {
            Operations.finish(db, opId, "FAILED", "OPERATION_INTERRUPTED", "连接已变化", null);
            return false;
        }
        return "VERIFYING".equals(op.state);
    }

    private void finishVerificationFailure(String ws, String opId, long generation, String code, String message) {
        synchronized (loginLock) {
            android.database.sqlite.SQLiteDatabase writer = db.getWritableDatabase();
            writer.beginTransaction();
            try {
                if (verificationCurrent(ws, opId, generation)) {
                    // A failed replacement must keep an existing working session.
                    ConnectionRow.markError(db, ws, code, message, null);
                    Operations.finish(db, opId, "FAILED", code, message, null);
                }
                writer.setTransactionSuccessful();
            } finally { writer.endTransaction(); }
        }
    }

    // ------------------------------------------------------------------
    // Fetch + preview
    // ------------------------------------------------------------------

    public JSONObject fetchPreview(final String ws, String semester, long expectedRevision,
                                   String idempotencyKey) throws HenuError {
        synchronized (loginLock) {
        requireWorkspace(ws);
        if (expectedRevision < 0 || expectedRevision > 9007199254740991L) throw HenuError.invalid("版本无效");
        final String sem = requireSemester(semester);
        final String key = normalizeKey(idempotencyKey);
        final String digest = sha256("FETCH\n" + ws + "\n" + sem + "\n" + expectedRevision);
        String existing = Operations.findIdempotent(db, ws, key, digest);
        if (existing != null) return getOperation(ws, existing);
        Operations.ActiveOp active = Operations.ActiveOp.read(db, ws);
        if (active != null) {
            throw new HenuError("OPERATION_IN_PROGRESS", "已有进行中的任务");
        }
        long revision = db.workspaceRevision(ws);
        if (revision != expectedRevision) {
            JSONObject details = new JSONObject();
            try {
                details.put("expected_revision", expectedRevision);
                details.put("current_revision", revision);
            } catch (org.json.JSONException ignored) {
            }
            HenuError err = new HenuError("REVISION_CONFLICT", "本地数据已变化");
            return errView(err);
        }
        final ConnectionRow conn = ConnectionRow.read(db, ws);
        if (conn == null || conn.ciphertext == null) {
            throw new HenuError("AUTH_REQUIRED", "尚未连接河大");
        }
        if (!"CONNECTED".equals(conn.state) && !"TEMPORARY_FAILURE".equals(conn.state)) {
            throw new HenuError("UPSTREAM_AUTH_REQUIRED", "学校会话已失效");
        }
        final String opId = LocalDb.newUuid();
        long now = System.currentTimeMillis();
        Operations.insert(db, opId, ws, "FETCH", "QUEUED", key, digest, now, now + 60_000);
        worker.submit(() -> runFetch(ws, sem, opId, conn, expectedRevision));
        return getOperation(ws, opId);
        }
    }

    private void runFetch(String ws, String semester, String opId, ConnectionRow conn, long expectedRevision) {
        try {
            if (!Operations.transition(db, opId, "QUEUED", "RUNNING", null, null)) return;
            JSONObject payload = new JSONObject(new String(
                    SessionCrypto.open(conn.ciphertext, SessionCrypto.aad(ws, "henu", conn.generation)),
                    StandardCharsets.UTF_8));
            Map<String, String> cookies = new LinkedHashMap<>();
            JSONObject jar = payload.getJSONObject("cookies");
            java.util.Iterator<String> it = jar.keys();
            while (it.hasNext()) {
                String name = it.next();
                cookies.put(name, jar.getString(name));
            }
            Fetched fetched = fetcher.fetch(cookies);
            SchoolClient.Term term = fetched.term;
            // Only the experimentally verified term mapping is supported.
            if (!term.year.matches("[0-9]{4}") || !"0".equals(term.term) || !"第一学期".equals(term.label)
                    || !semester.equals(term.year + "S2") || !(term.year + "-" + (Integer.parseInt(term.year) + 1)
                    + "学年第一学期").equals(fetched.parsed.optString("semester_label")))
                throw new HenuError("UNRECOGNIZED_DOCUMENT", "学校返回的学期与目标不一致");
            String fingerprint = sha256(term.userCode);
            if (conn.subjectFingerprint == null || !conn.subjectFingerprint.equals(fingerprint))
                throw new HenuError("UPSTREAM_ACCOUNT_CHANGED", "学校账号与授权不一致");
            // Network and parsing are over. Publish the preview, rotated
            // session and task result together after rechecking the snapshot.
            synchronized (loginLock) {
            android.database.sqlite.SQLiteDatabase writer = db.getWritableDatabase();
            writer.beginTransaction();
            try {
            if (!fetchCurrent(ws, opId, conn.generation)) { writer.setTransactionSuccessful(); return; }
            if (db.workspaceRevision(ws) != expectedRevision) {
                Operations.finish(db, opId, "FAILED", "REVISION_CONFLICT", "本地数据已变化", null);
                writer.setTransactionSuccessful(); return;
            }
            String previewId = LocalDb.newUuid();
            JSONObject preview = com.xique.assistant.henu.parse.PreviewBuilder.build(
                    db, previewId, ws, semester, fetched.parsed,
                    expectedRevision, conn.generation,
                    System.currentTimeMillis() + PREVIEW_TTL_MS,
                    LocalSettingsStore.readStored(db, ws, semester));
            JSONObject sealedPayload = new JSONObject().put("cookies", new JSONObject(fetched.cookies))
                .put("user_code", term.userCode);
            ConnectionRow.upsertConnected(db, ws, conn.generation,
                SessionCrypto.seal(sealedPayload.toString().getBytes(StandardCharsets.UTF_8),
                    SessionCrypto.aad(ws, "henu", conn.generation)), fingerprint, term.year, term.term, term.label);
            Operations.finishWithPreview(db, opId, "SUCCEEDED", previewId);
            writer.setTransactionSuccessful();
            } finally { writer.endTransaction(); }
            }
        } catch (SchoolClient.SchoolException e) {
            finishFetchFailure(ws, opId, conn.generation, e.code, e.getMessage());
        } catch (HenuError e) {
            finishFetchFailure(ws, opId, conn.generation, e.code, e.getMessage());
        } catch (Exception e) {
            finishFetchFailure(ws, opId, conn.generation, "STORAGE_UNAVAILABLE", "本地存储失败");
        }
    }

    private boolean fetchCurrent(String ws, String opId, long generation) {
        Operations.OpRow op = Operations.get(db, ws, opId);
        if (op == null || isTerminal(op.state)) return false;
        if (op.expiresAt <= System.currentTimeMillis()) {
            Operations.finish(db, opId, "EXPIRED", null, null, null); return false;
        }
        ConnectionRow row = ConnectionRow.read(db, ws);
        if (row == null || row.generation != generation || row.ciphertext == null) {
            Operations.finish(db, opId, "FAILED", "OPERATION_INTERRUPTED", "连接已变化", null); return false;
        }
        return "RUNNING".equals(op.state);
    }

    private void finishFetchFailure(String ws, String id, long generation, String code, String message) {
        synchronized (loginLock) {
            android.database.sqlite.SQLiteDatabase writer = db.getWritableDatabase(); writer.beginTransaction();
            try {
                if (fetchCurrent(ws, id, generation)) {
                    String state = "UPSTREAM_AUTH_REQUIRED".equals(code) ? "AUTH_REQUIRED"
                        : "UPSTREAM_UNAVAILABLE".equals(code) ? "TEMPORARY_FAILURE" : null;
                    ConnectionRow.markError(db, ws, code, message, state);
                    Operations.finish(db, id, "FAILED", code, message, null);
                }
                writer.setTransactionSuccessful();
            } finally { writer.endTransaction(); }
        }
    }

    public JSONObject getPreview(String ws, String previewId) throws HenuError {
        requireWorkspace(ws);
        Previews.sweep(db, System.currentTimeMillis());
        Long expiry = Previews.expiresAt(db, ws, previewId);
        if (expiry != null && expiry <= System.currentTimeMillis()) throw new HenuError("PREVIEW_EXPIRED", "预览已过期，请重新检查");
        String payload = Previews.payload(db, ws, previewId);
        if (payload == null) throw HenuError.notFound("预览不存在");
        try {
            JSONObject value = new JSONObject(payload);
            ConnectionRow conn = ConnectionRow.read(db, ws);
            boolean fresh = value.getLong("base_revision") == db.workspaceRevision(ws)
                && value.getLong("connection_generation") == (conn == null ? 0 : conn.generation);
            value.put("validation", new JSONObject().put("state", fresh ? "FRESH" : "STALE"));
            if (!fresh) value.put("can_commit", false);
            return value;
        } catch (org.json.JSONException e) {
            throw HenuError.storage("预览损坏");
        }
    }

    // ------------------------------------------------------------------
    // Operations / disconnect
    // ------------------------------------------------------------------

    public JSONObject getOperation(String ws, String opId) throws HenuError {
        requireWorkspace(ws);
        Operations.OpRow op = Operations.get(db, ws, opId);
        if (op == null) throw HenuError.notFound("任务不存在");
        if (!isTerminal(op.state) && System.currentTimeMillis() >= op.expiresAt) {
            Operations.transition(db, opId, op.state, "EXPIRED", null, null);
            op = Operations.get(db, ws, opId);
        }
        return operationView(op);
    }

    private JSONObject operationView(Operations.OpRow op) throws HenuError {
        JSONObject out = new JSONObject();
        try {
            out.put("operation_id", op.id);
            out.put("kind", op.kind);
            out.put("state", op.state);
            out.put("created_at", iso(op.createdAt));
            out.put("expires_at", iso(op.expiresAt));
            out.put("finished_at", op.finishedAt == null ? JSONObject.NULL : iso(op.finishedAt));
            out.put("preview_id", op.previewId == null ? JSONObject.NULL : op.previewId);
            if (op.errorCode != null) {
                JSONObject err = new JSONObject();
                err.put("code", op.errorCode);
                err.put("message", op.errorMessage == null ? "" : op.errorMessage);
                err.put("occurred_at", op.finishedAt == null ? iso(0) : iso(op.finishedAt));
                out.put("error", err);
            } else {
                out.put("error", JSONObject.NULL);
            }
        } catch (org.json.JSONException e) {
            throw HenuError.storage("序列化失败");
        }
        return out;
    }

    private JSONObject errView(HenuError err) throws HenuError {
        throw err;
    }

    public JSONObject cancelOperation(String ws, String opId) throws HenuError {
        synchronized (loginLock) {
        requireWorkspace(ws);
        Operations.OpRow op = Operations.get(db, ws, opId);
        if (op == null) throw HenuError.notFound("任务不存在");
        if (!isTerminal(op.state)) {
            Operations.transition(db, opId, op.state, "CANCELLED", null, null);
            if (opId.equals(pendingLoginOpId)) {
                pendingLoginOpId = null;
                pendingLoginNonce = null;
                loginCandidates.clear();
            }
        }
        JSONObject out = new JSONObject();
        try {
            out.put("cancelled", true);
        } catch (org.json.JSONException e) {
            throw HenuError.storage("序列化失败");
        }
        return out;
        }
    }

    public JSONObject disconnect(String ws, long expectedGeneration) throws HenuError {
        synchronized (loginLock) {
        requireWorkspace(ws);
        android.database.sqlite.SQLiteDatabase writer = db.getWritableDatabase();
        writer.beginTransaction();
        try {
        ConnectionRow row = ConnectionRow.read(db, ws);
        long current = row == null ? 0 : row.generation;
        if (current != expectedGeneration) {
            throw new HenuError("REVISION_CONFLICT", "连接代际已变化");
        }
        long now = System.currentTimeMillis();
        Operations.cancelActive(db, ws, now);
        ConnectionRow.disconnect(db, ws, current + 1);
        pendingLoginOpId = null;
        pendingLoginNonce = null;
        loginCandidates.clear();
        JSONObject out = new JSONObject();
        try {
            out.put("connection_generation", current + 1);
            out.put("connection_state", "DISCONNECTED");
        } catch (org.json.JSONException e) {
            throw HenuError.storage("序列化失败");
        }
        writer.setTransactionSuccessful();
        return out;
        } finally { writer.endTransaction(); }
        }
    }

    // ------------------------------------------------------------------
    // helpers
    // ------------------------------------------------------------------

    static boolean isTerminal(String state) {
        return state.equals("SUCCEEDED") || state.equals("FAILED")
                || state.equals("CANCELLED") || state.equals("EXPIRED");
    }

    private void requireWorkspace(String ws) throws HenuError {
        if (ws == null || !ws.equals(db.ensureWorkspace())) {
            throw HenuError.notFound("工作区不存在");
        }
        db.ensureWorkspace();
    }

    private String requireSemester(String semester) throws HenuError {
        if (semester == null || !semester.matches("[A-Za-z0-9_-]{1,64}")) {
            throw HenuError.invalid("semester 不合法");
        }
        return semester;
    }

    private String normalizeKey(String key) throws HenuError {
        if (key == null) throw HenuError.invalid("idempotency_key 必填");
        String k = key.trim().toLowerCase(Locale.ROOT);
        if (!k.matches("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")) {
            throw HenuError.invalid("idempotency_key 必须是 UUID");
        }
        return k;
    }

    public JSONObject getSettings(String ws, String semester) throws HenuError {
        requireWorkspace(ws);
        return new LocalSettingsStore(db).get(ws, semester);
    }

    public JSONObject putSettings(String ws, long expectedRevision, JSONObject settings)
            throws HenuError {
        requireWorkspace(ws);
        return new LocalSettingsStore(db).put(ws, expectedRevision, settings);
    }

    /** Personal display override only; source sessions and school data are untouched. */
    public JSONObject setCourseColor(String ws, long courseId, String color) throws HenuError {
        requireWorkspace(ws);
        if (courseId <= 0) throw HenuError.invalid("课程 ID 无效");
        String normalized = color == null ? "" : color.trim().toUpperCase(Locale.ROOT);
        if (!normalized.matches("#[0-9A-F]{6}")) throw HenuError.invalid("颜色格式无效");
        SQLiteDatabase writer = db.getWritableDatabase();
        writer.beginTransaction();
        try {
            ContentValues values = new ContentValues();
            values.put("color", normalized);
            values.put("updated_at", iso(System.currentTimeMillis()));
            int updated = writer.update("local_courses", values,
                    "workspace_id=? AND id=? AND active=1",
                    new String[]{ws, String.valueOf(courseId)});
            if (updated != 1) throw HenuError.notFound("课程不存在");
            long revision = db.workspaceRevision(ws) + 1;
            ContentValues workspace = new ContentValues();
            workspace.put("revision", revision);
            if (writer.update("local_workspaces", workspace, "id=?", new String[]{ws}) != 1) {
                throw HenuError.notFound("工作区不存在");
            }
            writer.setTransactionSuccessful();
            try {
                return new JSONObject().put("revision", revision).put("color", normalized);
            } catch (org.json.JSONException e) {
                throw HenuError.storage("颜色结果序列化失败");
            }
        } finally {
            writer.endTransaction();
        }
    }

    public JSONObject saveCourse(String ws, long expectedRevision, long courseId, String syncId, JSONObject course) throws HenuError {
        requireWorkspace(ws);
        return new com.xique.assistant.henu.storage.LocalCourseStore(db).save(ws, expectedRevision, courseId, syncId, course);
    }

    public JSONObject deleteCourse(String ws, long expectedRevision, long courseId) throws HenuError {
        requireWorkspace(ws);
        return new com.xique.assistant.henu.storage.LocalCourseStore(db).delete(ws, expectedRevision, courseId);
    }

    public boolean importLegacySnapshot(JSONObject snapshot, long generation) throws HenuError {
        return LegacyImport.importSnapshot(db, snapshot, generation);
    }

    public JSONObject commitPreview(String ws, String previewId, long expectedRevision,
                                    String idempotencyKey, JSONArray acknowledged)
            throws HenuError {
        requireWorkspace(ws);
        return CommitService.commit(db, ws, previewId, expectedRevision,
                idempotencyKey, acknowledged);
    }

    public static String iso(long millis) {
        if (millis <= 0) return "1970-01-01T00:00:00Z";
        return new java.text.SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.ROOT) {{
            setTimeZone(java.util.TimeZone.getTimeZone("UTC"));
        }}.format(new java.util.Date(millis));
    }

    public static String sha256(String value) {
        try {
            MessageDigest md = MessageDigest.getInstance("SHA-256");
            byte[] d = md.digest(value.getBytes(StandardCharsets.UTF_8));
            StringBuilder sb = new StringBuilder();
            for (byte b : d) sb.append(String.format("%02x", b));
            return sb.toString();
        } catch (Exception e) {
            throw new IllegalStateException("sha-256 unavailable");
        }
    }
}
