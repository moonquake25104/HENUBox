package com.xique.assistant.henu.storage;

import android.content.ContentValues;
import android.database.Cursor;

import com.xique.assistant.henu.ops.HenuError;

import org.json.JSONObject;

/**
 * Commit receipts and request-key aliases. A receipt is immutable for its
 * 30-day retention. A committed preview may be retried with a new key: the
 * alias maps that key to the original receipt without changing its payload.
 */
public final class Receipts {

    public static final long RETENTION_MS = 30L * 24 * 60 * 60 * 1000;

    public static final class Stored {
        public final String requestSha256;
        public final JSONObject receipt;
        public final long createdAt;
        public final long expiresAt;

        Stored(String requestSha256, JSONObject receipt, long createdAt, long expiresAt) {
            this.requestSha256 = requestSha256;
            this.receipt = receipt;
            this.createdAt = createdAt;
            this.expiresAt = expiresAt;
        }
    }

    private Receipts() {
    }

    /** Replay lookup. Same key + different request digest is a conflict. */
    public static JSONObject replay(LocalDb db, String ws, String key, String digest)
            throws HenuError {
        Stored stored = find(db, ws, key);
        if (stored == null) return null;
        if (!stored.requestSha256.equals(digest)) {
            throw new HenuError("IDEMPOTENCY_CONFLICT", "此请求键已用于其他操作");
        }
        return stored.receipt;
    }

    public static Stored find(LocalDb db, String ws, String key) throws HenuError {
        Cursor c = db.getReadableDatabase().query("local_receipts",
                new String[]{"request_sha256", "receipt_json", "created_at", "expires_at"},
                "workspace_id=? AND idempotency_key=?", new String[]{ws, key},
                null, null, null);
        try {
            if (c.moveToFirst()) return read(c);
        } finally {
            c.close();
        }
        c = db.getReadableDatabase().rawQuery("SELECT r.request_sha256,r.receipt_json,"
                + "r.created_at,r.expires_at FROM local_receipts r"
                + " JOIN local_receipt_aliases a ON a.workspace_id=r.workspace_id"
                + " AND a.commit_id=r.commit_id WHERE r.workspace_id=? AND a.idempotency_key=?",
                new String[]{ws, key});
        try {
            if (c.moveToFirst()) return read(c);
        } finally {
            c.close();
        }
        return null;
    }

    public static Stored byCommit(LocalDb db, String ws, String commitId) throws HenuError {
        try (Cursor c = db.getReadableDatabase().query("local_receipts",
                new String[]{"request_sha256", "receipt_json", "created_at", "expires_at"},
                "workspace_id=? AND commit_id=?", new String[]{ws, commitId},
                null, null, null)) {
            return c.moveToFirst() ? read(c) : null;
        }
    }

    public static void insert(LocalDb db, String ws, String commitId, String key,
                              String requestSha256, JSONObject receipt,
                              long createdAt, long expiresAt) throws HenuError {
        ContentValues v = new ContentValues();
        v.put("workspace_id", ws);
        v.put("commit_id", commitId);
        v.put("idempotency_key", key);
        v.put("request_sha256", requestSha256);
        v.put("receipt_json", receipt.toString());
        v.put("created_at", createdAt);
        v.put("expires_at", expiresAt);
        try {
            db.getWritableDatabase().insertOrThrow("local_receipts", null, v);
        } catch (android.database.sqlite.SQLiteConstraintException e) {
            throw new HenuError("IDEMPOTENCY_CONFLICT", "此请求键已用于其他操作");
        } catch (RuntimeException e) { throw HenuError.storage("回执保存失败");
        }
    }

    public static void addAlias(LocalDb db, String ws, String key, String commitId, long now)
            throws HenuError {
        ContentValues v = new ContentValues();
        v.put("workspace_id", ws);
        v.put("idempotency_key", key);
        v.put("commit_id", commitId);
        v.put("created_at", now);
        try {
            db.getWritableDatabase().insertOrThrow("local_receipt_aliases", null, v);
        } catch (android.database.sqlite.SQLiteConstraintException e) {
            throw new HenuError("IDEMPOTENCY_CONFLICT", "此请求键已用于其他操作");
        } catch (RuntimeException e) { throw HenuError.storage("请求键保存失败");
        }
    }

    public static void sweep(LocalDb db, long now) {
        db.getWritableDatabase().execSQL("DELETE FROM local_receipt_aliases WHERE NOT EXISTS"
                + " (SELECT 1 FROM local_receipts r WHERE r.workspace_id=local_receipt_aliases.workspace_id"
                + " AND r.commit_id=local_receipt_aliases.commit_id)");
        db.getWritableDatabase().delete("local_receipts", "expires_at<=?",
                new String[]{String.valueOf(now)});
    }

    private static Stored read(Cursor c) throws HenuError {
        try {
            return new Stored(c.getString(0), new JSONObject(c.getString(1)),
                    c.getLong(2), c.getLong(3));
        } catch (org.json.JSONException e) {
            throw HenuError.storage("回执损坏");
        }
    }
}
