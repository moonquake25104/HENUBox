package com.xique.assistant.henu.storage;

import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;
import android.database.sqlite.SQLiteOpenHelper;

import java.util.UUID;

/**
 * The local-v2 database (no_backup/local-v2.sqlite). Schema 4 adds per-row
 * personal edits without modifying school sources. Schema 3 adds semester
 * settings, receipts, migration markers, local course projections and the
 * source-session rows used by later previews. The main process is the only
 * writer; the auth process never touches this file. v1 files
 * (timetable-v1.sqlite, encrypted desktop session) are never opened here.
 */
public final class LocalDb extends SQLiteOpenHelper {

    public static final String DB_NAME = "local-v2.sqlite";
    public static final int VERSION = 4;

    public LocalDb(Context context) {
        super(context.getApplicationContext(), preparePath(context), null, VERSION);
    }

    /** Isolated device tests: never open the probe's real school-session DB. */
    public LocalDb(Context context, String testName) {
        super(context.getApplicationContext(), testPath(context, testName), null, VERSION);
    }

    private static String testPath(Context context, String name) {
        if (name == null || !name.matches("test-[a-z0-9-]+\\.sqlite")) throw new IllegalArgumentException();
        return new java.io.File(context.getNoBackupFilesDir(), name).getAbsolutePath();
    }

    /** Copy the original M1 database once. Keep its encrypted original intact.
     * A WAL checkpoint and fsync precede the atomic rename; a partial copy can
     * never become the live database. No secrets leave this app's private UID. */
    private static synchronized String preparePath(Context context) {
        java.io.File target = new java.io.File(context.getNoBackupFilesDir(), DB_NAME);
        java.io.File old = context.getApplicationContext().getDatabasePath(DB_NAME);
        if (!target.exists() && old.exists() && !old.equals(target)) {
            java.io.File staging = new java.io.File(target.getPath() + ".migrating");
            try {
                try (SQLiteDatabase source = SQLiteDatabase.openDatabase(old.getPath(), null, SQLiteDatabase.OPEN_READWRITE);
                     Cursor check = source.rawQuery("PRAGMA wal_checkpoint(TRUNCATE)", null)) {
                    if (check.moveToFirst() && check.getInt(0) != 0) throw new IllegalStateException("database busy");
                }
                try (java.io.FileInputStream in = new java.io.FileInputStream(old);
                     java.io.FileOutputStream out = new java.io.FileOutputStream(staging)) {
                    byte[] buffer = new byte[65536]; int count;
                    while ((count = in.read(buffer)) >= 0) out.write(buffer, 0, count);
                    out.getFD().sync();
                }
                try (SQLiteDatabase copy = SQLiteDatabase.openDatabase(staging.getPath(), null, SQLiteDatabase.OPEN_READONLY);
                     Cursor check = copy.rawQuery("PRAGMA quick_check", null)) {
                    if (!check.moveToFirst() || !"ok".equals(check.getString(0))) throw new IllegalStateException("invalid copy");
                }
                if (!staging.renameTo(target)) throw new IllegalStateException("migration rename failed");
            } catch (Exception e) { throw new IllegalStateException("Local database relocation failed"); }
        }
        return target.getAbsolutePath();
    }

    @Override
    public void onCreate(SQLiteDatabase db) {
        db.execSQL("CREATE TABLE local_workspaces ("
                + "id TEXT PRIMARY KEY,"
                + "revision INTEGER NOT NULL DEFAULT 0,"
                + "next_course_id INTEGER NOT NULL DEFAULT 1,"
                + "created_at INTEGER NOT NULL)");
        db.execSQL("CREATE TABLE henu_connections ("
                + "workspace_id TEXT PRIMARY KEY,"
                + "school TEXT NOT NULL DEFAULT 'henu',"
                + "generation INTEGER NOT NULL DEFAULT 0,"
                + "state TEXT NOT NULL DEFAULT 'DISCONNECTED',"
                + "subject_fingerprint TEXT,"
                + "session_ciphertext BLOB,"
                + "term_year TEXT, term_term TEXT, term_label TEXT,"
                + "verified_at INTEGER, last_error_code TEXT, last_error_message TEXT, last_error_at INTEGER,"
                + "updated_at INTEGER NOT NULL)");
        db.execSQL("CREATE TABLE local_operations ("
                + "id TEXT PRIMARY KEY,"
                + "workspace_id TEXT NOT NULL,"
                + "kind TEXT NOT NULL,"
                + "state TEXT NOT NULL,"
                + "idempotency_key TEXT,"
                + "request_sha256 TEXT,"
                + "created_at INTEGER NOT NULL,"
                + "expires_at INTEGER NOT NULL,"
                + "started_at INTEGER, finished_at INTEGER,"
                + "preview_id TEXT, error_code TEXT, error_message TEXT,"
                + "UNIQUE(workspace_id, idempotency_key))");
        db.execSQL("CREATE TABLE local_previews ("
                + "id TEXT PRIMARY KEY,"
                + "workspace_id TEXT NOT NULL,"
                + "semester TEXT NOT NULL,"
                + "status TEXT NOT NULL,"
                + "base_revision INTEGER NOT NULL,"
                + "connection_generation INTEGER NOT NULL,"
                + "expires_at INTEGER NOT NULL,"
                + "payload TEXT NOT NULL,purged_at INTEGER,"
                + "committed_at INTEGER,commit_id TEXT,"
                + "created_at INTEGER NOT NULL)");
        createSchema3Objects(db);
        createSchema4Objects(db);
        db.execSQL("CREATE INDEX IF NOT EXISTS idx_operations_active ON local_operations(workspace_id, state)");
    }

    @Override
    public void onUpgrade(SQLiteDatabase db, int oldVersion, int newVersion) {
        if (newVersion != VERSION || oldVersion < 1) {
            throw new IllegalStateException("Unsupported local schema");
        }
        // SQLiteOpenHelper wraps onCreate/onUpgrade in one database
        // transaction. Every step below therefore either commits together or
        // rolls back to the untouched pre-upgrade file.
        if (oldVersion == 1) upgradeV1ToV2(db);
        if (oldVersion <= 2) createSchema3Objects(db);
        if (oldVersion <= 3) createSchema4Objects(db);
    }

    private static void createSchema4Objects(SQLiteDatabase db) {
        db.execSQL("CREATE TABLE IF NOT EXISTS local_course_overrides ("
                + "workspace_id TEXT NOT NULL,course_id INTEGER NOT NULL,fields_json TEXT NOT NULL,"
                + "PRIMARY KEY(workspace_id,course_id))");
    }

    /** Schema 3 additions. IF NOT EXISTS also makes recovery idempotent when a
     * device test reconstructed a v1 marker on top of the current file. */
    private static void createSchema3Objects(SQLiteDatabase db) {
        db.execSQL("CREATE TABLE IF NOT EXISTS local_semesters ("
                + "workspace_id TEXT NOT NULL,"
                + "semester TEXT NOT NULL,"
                + "revision INTEGER NOT NULL DEFAULT 0,"
                + "settings_json TEXT NOT NULL,"
                + "updated_at INTEGER NOT NULL,"
                + "PRIMARY KEY(workspace_id, semester))");
        db.execSQL("CREATE TABLE IF NOT EXISTS local_receipts ("
                + "workspace_id TEXT NOT NULL,"
                + "commit_id TEXT NOT NULL,"
                + "idempotency_key TEXT NOT NULL,"
                + "request_sha256 TEXT NOT NULL,"
                + "receipt_json TEXT NOT NULL,"
                + "created_at INTEGER NOT NULL,"
                + "expires_at INTEGER NOT NULL,"
                + "PRIMARY KEY(workspace_id, commit_id),"
                + "UNIQUE(workspace_id, idempotency_key))");
        // A committed preview may be retried with a fresh request key. The
        // receipt row remains authoritative; aliases only add replay keys.
        db.execSQL("CREATE TABLE IF NOT EXISTS local_receipt_aliases ("
                + "workspace_id TEXT NOT NULL,"
                + "idempotency_key TEXT NOT NULL,"
                + "commit_id TEXT NOT NULL,"
                + "created_at INTEGER NOT NULL,"
                + "PRIMARY KEY(workspace_id, idempotency_key))");
        db.execSQL("CREATE TABLE IF NOT EXISTS local_migrations ("
                + "name TEXT PRIMARY KEY,"
                + "completed_at INTEGER NOT NULL)");
        db.execSQL("CREATE TABLE IF NOT EXISTS local_courses ("
                + "workspace_id TEXT NOT NULL,"
                + "id INTEGER NOT NULL,"
                + "semester TEXT NOT NULL,"
                + "sync_id TEXT NOT NULL,"
                + "name TEXT NOT NULL,"
                + "day_of_week INTEGER NOT NULL,"
                + "start_time TEXT NOT NULL,"
                + "end_time TEXT NOT NULL,"
                + "week_pattern TEXT NOT NULL,"
                + "semester_start_date TEXT NOT NULL,"
                + "location TEXT NOT NULL DEFAULT '',"
                + "teacher TEXT NOT NULL DEFAULT '',"
                + "color TEXT NOT NULL,"
                + "source_kind TEXT NOT NULL,"
                + "source_entry_id TEXT,"
                + "source_course_id TEXT,"
                + "active INTEGER NOT NULL DEFAULT 1,"
                + "created_at TEXT NOT NULL,"
                + "updated_at TEXT NOT NULL,"
                + "PRIMARY KEY(workspace_id, id),"
                + "UNIQUE(workspace_id, sync_id))");
        db.execSQL("CREATE TABLE IF NOT EXISTS local_source_sessions ("
                + "workspace_id TEXT NOT NULL,"
                + "session_id TEXT NOT NULL,"
                + "course_id TEXT NOT NULL,"
                + "entry_id TEXT NOT NULL,"
                + "content_sha TEXT NOT NULL,"
                + "source_json TEXT NOT NULL,"
                + "active INTEGER NOT NULL DEFAULT 1,"
                + "created_at TEXT NOT NULL,"
                + "PRIMARY KEY(workspace_id, session_id))");
        db.execSQL("CREATE INDEX IF NOT EXISTS idx_courses_semester ON local_courses(workspace_id, semester, active)");
        db.execSQL("CREATE INDEX IF NOT EXISTS idx_courses_source_entry ON local_courses(workspace_id, source_entry_id)");
        db.execSQL("CREATE INDEX IF NOT EXISTS idx_source_sessions_course ON local_source_sessions(workspace_id, course_id, active)");
        db.execSQL("CREATE INDEX IF NOT EXISTS idx_receipts_expiry ON local_receipts(workspace_id, expires_at)");
        addColumnIfMissing(db, "local_previews", "purged_at", "INTEGER");
        addColumnIfMissing(db, "local_previews", "committed_at", "INTEGER");
        addColumnIfMissing(db, "local_previews", "commit_id", "TEXT");
    }

    private static void addColumnIfMissing(SQLiteDatabase db, String table, String column, String type) {
        try (Cursor c = db.rawQuery("PRAGMA table_info(" + table + ")", null)) {
            int name = c.getColumnIndexOrThrow("name");
            while (c.moveToNext()) if (column.equals(c.getString(name))) return;
        }
        db.execSQL("ALTER TABLE " + table + " ADD COLUMN " + column + " " + type);
    }

    /** Exact schema 1 -> 2 migration shipped by M1; unchanged except for being
     * called from the version-dispatching onUpgrade. */
    private static void upgradeV1ToV2(SQLiteDatabase db) {
        // Rebind ciphertext
        // AAD before changing the workspace key, otherwise existing logins die.
        String next = newUuid();
        try (Cursor c = db.rawQuery("SELECT generation,session_ciphertext FROM henu_connections WHERE workspace_id='default'", null)) {
            if (c.moveToFirst() && !c.isNull(1)) {
                long gen = c.getLong(0);
                try {
                    byte[] clear = com.xique.assistant.henu.crypto.SessionCrypto.open(c.getBlob(1),
                        com.xique.assistant.henu.crypto.SessionCrypto.aad("default", "henu", gen));
                    byte[] sealed;
                    try { sealed = com.xique.assistant.henu.crypto.SessionCrypto.seal(clear,
                        com.xique.assistant.henu.crypto.SessionCrypto.aad(next, "henu", gen)); }
                    finally { java.util.Arrays.fill(clear, (byte) 0); }
                    ContentValues v = new ContentValues(); v.put("session_ciphertext", sealed);
                    db.update("henu_connections", v, "workspace_id='default'", null);
                } catch (Exception e) { throw new IllegalStateException("Session migration failed"); }
            }
        }
        for (String table : new String[]{"local_workspaces", "henu_connections", "local_operations", "local_previews"}) {
            String column = table.equals("local_workspaces") ? "id" : "workspace_id";
            db.execSQL("UPDATE " + table + " SET " + column + "=? WHERE " + column + "='default'", new Object[]{next});
        }
        db.execSQL("ALTER TABLE local_previews ADD COLUMN purged_at INTEGER");
        // Old probe preview IDs were array offsets. They are local references,
        // never source identities; remap every occurrence in the saved DTO.
        try (Cursor c = db.rawQuery("SELECT id,payload FROM local_previews", null)) {
            while (c.moveToNext()) {
                try {
                    org.json.JSONObject payload = new org.json.JSONObject(c.getString(1));
                    java.util.Map<String, String> ids = new java.util.HashMap<>();
                    org.json.JSONArray courses = payload.optJSONArray("courses");
                    if (courses != null) for (int i = 0; i < courses.length(); i++) {
                        org.json.JSONObject course = courses.getJSONObject(i);
                        ids.put(course.getString("preview_course_id"), newUuid());
                        org.json.JSONArray sessions = course.getJSONArray("sessions");
                        for (int j = 0; j < sessions.length(); j++) ids.put(
                            sessions.getJSONObject(j).getString("preview_session_id"), newUuid());
                    }
                    remapIds(payload, ids);
                    ContentValues v = new ContentValues(); v.put("payload", payload.toString());
                    db.update("local_previews", v, "id=?", new String[]{c.getString(0)});
                } catch (org.json.JSONException e) { throw new IllegalStateException("Preview migration failed"); }
            }
        }
    }

    private static void remapIds(Object node, java.util.Map<String, String> ids) throws org.json.JSONException {
        if (node instanceof org.json.JSONObject) {
            org.json.JSONObject object = (org.json.JSONObject) node;
            java.util.Iterator<String> keys = object.keys();
            while (keys.hasNext()) {
                String key = keys.next(); Object value = object.get(key);
                if (value instanceof String && ids.containsKey(value)) object.put(key, ids.get(value));
                else remapIds(value, ids);
            }
        } else if (node instanceof org.json.JSONArray) {
            org.json.JSONArray array = (org.json.JSONArray) node;
            for (int i = 0; i < array.length(); i++) {
                Object value = array.get(i);
                if (value instanceof String && ids.containsKey(value)) array.put(i, ids.get(value));
                else remapIds(value, ids);
            }
        }
    }

    /** Creates the default workspace if missing and returns its id. */
    public String ensureWorkspace() {
        SQLiteDatabase db = getWritableDatabase();
        long now = System.currentTimeMillis();
        db.beginTransaction();
        try {
            Cursor c = db.query("local_workspaces", new String[]{"id"}, null,
                    null, null, null, null);
            boolean exists = c.moveToFirst();
            String id = exists ? c.getString(0) : newUuid();
            c.close();
            if (!exists) {
                ContentValues v = new ContentValues();
                v.put("id", id);
                v.put("revision", 0L);
                v.put("next_course_id", 1L);
                v.put("created_at", now);
                db.insertOrThrow("local_workspaces", null, v);
            }
            db.setTransactionSuccessful();
            return id;
        } finally {
            db.endTransaction();
        }
    }

    public long workspaceRevision(String ws) {
        SQLiteDatabase db = getReadableDatabase();
        Cursor c = db.query("local_workspaces", new String[]{"revision"}, "id=?",
                new String[]{ws}, null, null, null);
        try {
            if (!c.moveToFirst()) throw new IllegalStateException("workspace missing: " + ws);
            return c.getLong(0);
        } finally {
            c.close();
        }
    }

    public static String newUuid() {
        return UUID.randomUUID().toString().toLowerCase();
    }
}
