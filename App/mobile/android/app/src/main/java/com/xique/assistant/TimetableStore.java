package com.xique.assistant;

import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;
import java.io.File;
import org.json.JSONObject;

/** A complete, account-bound snapshot. SQLite replacement is one atomic transaction. */
final class TimetableStore {
    private final File file;
    private final SessionVault session;

    TimetableStore(Context context) { this(context, "timetable-v1", new SessionVault(context)); }
    TimetableStore(Context context, String namespace, SessionVault session) {
        if (!namespace.matches("[a-z0-9-]+")) throw new IllegalArgumentException();
        file = new File(context.getNoBackupFilesDir(), namespace + ".sqlite");
        this.session = session;
    }

    private SQLiteDatabase open() {
        SQLiteDatabase db = SQLiteDatabase.openOrCreateDatabase(file, null);
        db.execSQL("CREATE TABLE IF NOT EXISTS snapshot (id INTEGER PRIMARY KEY CHECK(id=1), origin TEXT NOT NULL, account TEXT NOT NULL, payload TEXT NOT NULL)");
        return db;
    }

    private JSONObject owner(long expected) throws Exception {
        JSONObject value = session.read();
        if (value.getLong("generation") != expected || value.isNull("session"))
            throw new SessionVault.Failure("SESSION_CHANGED");
        return value.getJSONObject("session");
    }

    JSONObject read(long expected) throws Exception {
        synchronized (SessionVault.LOCK) {
            if (!file.exists()) return null;
            JSONObject owner = owner(expected);
            try (SQLiteDatabase db = SQLiteDatabase.openDatabase(file.getAbsolutePath(),null,SQLiteDatabase.OPEN_READONLY); Cursor row = db.query("snapshot", new String[]{"payload"},
                "id=1 AND origin=? AND account=?", new String[]{owner.getString("server_origin"), owner.getString("account_id")}, null, null, null)) {
                return row.moveToFirst() ? new JSONObject(row.getString(0)) : null;
            }
        }
    }

    void write(long expected, JSONObject snapshot) throws Exception {
        String payload = snapshot.toString();
        if (payload.length() > 2_000_000 || snapshot.length() != 3 || snapshot.getInt("schema") != 1
            || !snapshot.getString("saved_at").matches("\\d{4}-\\d{2}-\\d{2}T.*Z")
            || snapshot.getJSONArray("courses").length() > 10000) throw new IllegalArgumentException();
        synchronized (SessionVault.LOCK) {
            JSONObject owner = owner(expected);
            try (SQLiteDatabase db = open()) {
                db.beginTransaction();
                try {
                    ContentValues row = new ContentValues();
                    row.put("id", 1); row.put("origin", owner.getString("server_origin"));
                    row.put("account", owner.getString("account_id")); row.put("payload", payload);
                    db.replaceOrThrow("snapshot", null, row);
                    db.setTransactionSuccessful();
                } finally { db.endTransaction(); }
            }
        }
    }

    void clear() {
        synchronized (SessionVault.LOCK) {
            try (SQLiteDatabase db = open()) { db.delete("snapshot", null, null); }
        }
    }
}
