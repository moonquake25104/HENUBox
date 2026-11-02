package com.xique.assistant.henu.storage;

import android.content.ContentValues;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;

/** Operation rows: guarded transitions so late executors can never revive a
 * cancelled or superseded task. */
public final class Operations {

    public static final class OpRow {
        public final String id;
        public final String kind;
        public final String state;
        public final long createdAt;
        public final long expiresAt;
        public final Long finishedAt;
        public final String previewId;
        public final String errorCode;
        public final String errorMessage;

        OpRow(String id, String kind, String state, long createdAt, long expiresAt,
              Long finishedAt, String previewId, String errorCode, String errorMessage) {
            this.id = id;
            this.kind = kind;
            this.state = state;
            this.createdAt = createdAt;
            this.expiresAt = expiresAt;
            this.finishedAt = finishedAt;
            this.previewId = previewId;
            this.errorCode = errorCode;
            this.errorMessage = errorMessage;
        }
    }

    public static final class ActiveOp {
        public final String id;
        public final String kind;

        ActiveOp(String id, String kind) {
            this.id = id;
            this.kind = kind;
        }

        public static ActiveOp read(LocalDb db, String ws) {
            expire(db, ws, System.currentTimeMillis());
            Cursor c = db.getReadableDatabase().query("local_operations",
                    new String[]{"id", "kind"},
                    "workspace_id=? AND state IN ('WAITING_FOR_USER','VERIFYING','QUEUED','RUNNING')",
                    new String[]{ws}, null, null, "created_at DESC", "1");
            try {
                if (!c.moveToFirst()) return null;
                return new ActiveOp(c.getString(0), c.getString(1));
            } finally {
                c.close();
            }
        }
    }

    public static void insert(LocalDb db, String id, String ws, String kind, String state,
                              String idemKey, String requestSha, long createdAt, long expiresAt) {
        ContentValues v = new ContentValues();
        v.put("id", id);
        v.put("workspace_id", ws);
        v.put("kind", kind);
        v.put("state", state);
        v.put("idempotency_key", idemKey);
        v.put("request_sha256", requestSha);
        v.put("created_at", createdAt);
        v.put("expires_at", expiresAt);
        db.getWritableDatabase().insertOrThrow("local_operations", null, v);
    }

    public static OpRow get(LocalDb db, String ws, String opId) {
        Cursor c = db.getReadableDatabase().query("local_operations", null,
                "workspace_id=? AND id=?", new String[]{ws, opId}, null, null, null);
        try {
            if (!c.moveToFirst()) return null;
            int fi = c.getColumnIndex("finished_at");
            int pi = c.getColumnIndex("preview_id");
            int ei = c.getColumnIndex("error_code");
            return new OpRow(opId,
                    c.getString(c.getColumnIndexOrThrow("kind")),
                    c.getString(c.getColumnIndexOrThrow("state")),
                    c.getLong(c.getColumnIndexOrThrow("created_at")),
                    c.getLong(c.getColumnIndexOrThrow("expires_at")),
                    c.isNull(fi) ? null : c.getLong(fi),
                    c.isNull(pi) ? null : c.getString(pi),
                    c.isNull(ei) ? null : c.getString(ei),
                    str(c, "error_message"));
        } finally {
            c.close();
        }
    }

    public static String findIdempotent(LocalDb db, String ws, String key) {
        Cursor c = db.getReadableDatabase().query("local_operations", new String[]{"id"},
                "workspace_id=? AND idempotency_key=?", new String[]{ws, key},
                null, null, "created_at DESC", "1");
        try {
            return c.moveToFirst() ? c.getString(0) : null;
        } finally {
            c.close();
        }
    }

    public static String findIdempotent(LocalDb db, String ws, String key, String digest)
            throws com.xique.assistant.henu.ops.HenuError {
        try (Cursor c = db.getReadableDatabase().query("local_operations", new String[]{"id", "request_sha256"},
                "workspace_id=? AND idempotency_key=?", new String[]{ws, key}, null, null, null)) {
            if (!c.moveToFirst()) return null;
            if (!digest.equals(c.getString(1))) throw new com.xique.assistant.henu.ops.HenuError(
                "IDEMPOTENCY_CONFLICT", "此请求键已用于其他操作");
            return c.getString(0);
        }
    }

    public static void expire(LocalDb db, String ws, long now) {
        db.getWritableDatabase().execSQL("UPDATE local_operations SET state='EXPIRED',finished_at=?"
            + " WHERE workspace_id=? AND expires_at<=? AND state IN ('WAITING_FOR_USER','VERIFYING','QUEUED','RUNNING')",
            new Object[]{now, ws, now});
    }

    /** State transition guarded by the expected current state; false if lost the race. */
    public static boolean transition(LocalDb db, String opId, String from, String to,
                                     String errorCode, String errorMessage) {
        ContentValues v = new ContentValues();
        v.put("state", to);
        if (errorCode != null) {
            v.put("error_code", errorCode);
            v.put("error_message", errorMessage);
        }
        if (isTerminalState(to)) v.put("finished_at", System.currentTimeMillis());
        if ("RUNNING".equals(to) || "VERIFYING".equals(to)) v.put("started_at", System.currentTimeMillis());
        return db.getWritableDatabase().update("local_operations", v,
                "id=? AND state=?", new String[]{opId, from}) == 1;
    }

    public static void finish(LocalDb db, String opId, String state, String errorCode,
                              String errorMessage, String previewId) {
        ContentValues v = new ContentValues();
        v.put("state", state);
        if (errorCode != null) {
            v.put("error_code", errorCode);
            v.put("error_message", errorMessage);
        }
        if (previewId != null) v.put("preview_id", previewId);
        v.put("finished_at", System.currentTimeMillis());
        db.getWritableDatabase().update("local_operations", v,
                "id=? AND state IN ('WAITING_FOR_USER','VERIFYING','QUEUED','RUNNING')", new String[]{opId});
    }

    public static void finishWithPreview(LocalDb db, String opId, String state, String previewId) {
        finish(db, opId, state, null, null, previewId);
    }

    public static void cancelActive(LocalDb db, String ws, long now) {
        ContentValues v = new ContentValues();
        v.put("state", "CANCELLED");
        v.put("finished_at", now);
        db.getWritableDatabase().update("local_operations", v,
                "workspace_id=? AND state IN ('WAITING_FOR_USER','VERIFYING','QUEUED','RUNNING')",
                new String[]{ws});
    }

    static boolean isTerminalState(String s) {
        return s.equals("SUCCEEDED") || s.equals("FAILED") || s.equals("CANCELLED")
                || s.equals("EXPIRED");
    }

    private static String str(Cursor c, String col) {
        int i = c.getColumnIndex(col);
        return i < 0 || c.isNull(i) ? null : c.getString(i);
    }
}
