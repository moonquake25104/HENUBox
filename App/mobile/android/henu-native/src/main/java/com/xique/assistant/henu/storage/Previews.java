package com.xique.assistant.henu.storage;

import android.content.ContentValues;
import android.database.Cursor;

import com.xique.assistant.henu.ops.HenuError;

/** Preview persistence: one immutable payload per preview row. */
public final class Previews {

    public static final class Row {
        public final String id;
        public final String semester;
        public final String status;
        public final long baseRevision;
        public final long connectionGeneration;
        public final long expiresAt;
        public final Long committedAt;
        public final String commitId;
        public final String payload;

        Row(String id, String semester, String status, long baseRevision,
            long connectionGeneration, long expiresAt, Long committedAt,
            String commitId, String payload) {
            this.id = id;
            this.semester = semester;
            this.status = status;
            this.baseRevision = baseRevision;
            this.connectionGeneration = connectionGeneration;
            this.expiresAt = expiresAt;
            this.committedAt = committedAt;
            this.commitId = commitId;
            this.payload = payload;
        }
    }

    /** Expired payloads contain private timetable data. Keep only a tombstone
     * for 30 days; operation/idempotency replay remains available independently. */
    public static void sweep(LocalDb db, long now) {
        android.database.sqlite.SQLiteDatabase writer = db.getWritableDatabase();
        writer.execSQL("UPDATE local_previews SET payload='{}',purged_at=? WHERE expires_at<=? AND purged_at IS NULL",
            new Object[]{now, now});
        writer.delete("local_previews", "purged_at IS NOT NULL AND expires_at<=?",
            new String[]{String.valueOf(now - 30L * 24 * 60 * 60 * 1000)});
        writer.delete("local_operations", "finished_at IS NOT NULL AND created_at<=?",
            new String[]{String.valueOf(now - 30L * 24 * 60 * 60 * 1000)});
        Receipts.sweep(db, now);
    }

    public static Long expiresAt(LocalDb db, String ws, String id) {
        try (android.database.Cursor c = db.getReadableDatabase().query("local_previews", new String[]{"expires_at"},
                "workspace_id=? AND id=?", new String[]{ws, id}, null, null, null)) {
            return c.moveToFirst() ? c.getLong(0) : null;
        }
    }

    public static void insert(LocalDb db, String id, String ws, String semester, String status,
                              long baseRevision, long connectionGeneration, long expiresAt,
                              String payloadJson) {
        ContentValues v = new ContentValues();
        v.put("id", id);
        v.put("workspace_id", ws);
        v.put("semester", semester);
        v.put("status", status);
        v.put("base_revision", baseRevision);
        v.put("connection_generation", connectionGeneration);
        v.put("expires_at", expiresAt);
        v.put("payload", payloadJson);
        v.put("created_at", System.currentTimeMillis());
        db.getWritableDatabase().insertOrThrow("local_previews", null, v);
    }

    public static String payload(LocalDb db, String ws, String previewId) {
        android.database.Cursor c = db.getReadableDatabase().query("local_previews",
                new String[]{"payload"}, "workspace_id=? AND id=?", new String[]{ws, previewId},
                null, null, null);
        try {
            return c.moveToFirst() ? c.getString(0) : null;
        } finally {
            c.close();
        }
    }

    public static Row read(LocalDb db, String ws, String previewId) throws HenuError {
        try (Cursor c = db.getReadableDatabase().query("local_previews",
                new String[]{"id", "semester", "status", "base_revision",
                        "connection_generation", "expires_at", "committed_at", "commit_id", "payload"},
                "workspace_id=? AND id=?", new String[]{ws, previewId}, null, null, null)) {
            if (!c.moveToFirst()) return null;
            int ci = c.getColumnIndexOrThrow("committed_at");
            int ii = c.getColumnIndexOrThrow("commit_id");
            return new Row(c.getString(c.getColumnIndexOrThrow("id")),
                    c.getString(c.getColumnIndexOrThrow("semester")),
                    c.getString(c.getColumnIndexOrThrow("status")),
                    c.getLong(c.getColumnIndexOrThrow("base_revision")),
                    c.getLong(c.getColumnIndexOrThrow("connection_generation")),
                    c.getLong(c.getColumnIndexOrThrow("expires_at")),
                    c.isNull(ci) ? null : c.getLong(ci),
                    c.isNull(ii) ? null : c.getString(ii),
                    c.getString(c.getColumnIndexOrThrow("payload")));
        } catch (RuntimeException e) {
            throw HenuError.storage("预览读取失败");
        }
    }

    /** Marks a preview committed and stores the COMMITTED status in the public
     * payload without changing its source snapshot. */
    public static void markCommitted(LocalDb db, String ws, String previewId,
                                     String commitId, String payloadJson, long now) {
        ContentValues v = new ContentValues();
        v.put("status", "COMMITTED");
        v.put("committed_at", now);
        v.put("commit_id", commitId);
        if (payloadJson != null) v.put("payload", payloadJson);
        db.getWritableDatabase().update("local_previews", v,
                "workspace_id=? AND id=? AND committed_at IS NULL",
                new String[]{ws, previewId});
    }
}
