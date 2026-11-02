package com.xique.assistant.henu.storage;

import android.content.ContentValues;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;

/** Typed access to the henu_connections row. Main process only. */
public final class ConnectionRow {

    public final long generation;
    public final String state;
    public final String subjectFingerprint;
    public final byte[] ciphertext;
    public final String termYear;
    public final String termTerm;
    public final String termLabel;
    public final Long verifiedAt;
    public final String errorCode;
    public final String errorMessage;
    public final Long errorAt;

    private ConnectionRow(long generation, String state, String subjectFingerprint, byte[] ciphertext,
                          String termYear, String termTerm, String termLabel, Long verifiedAt,
                          String errorCode, String errorMessage, Long errorAt) {
        this.generation = generation;
        this.state = state;
        this.subjectFingerprint = subjectFingerprint;
        this.ciphertext = ciphertext;
        this.termYear = termYear;
        this.termTerm = termTerm;
        this.termLabel = termLabel;
        this.verifiedAt = verifiedAt;
        this.errorCode = errorCode;
        this.errorMessage = errorMessage;
        this.errorAt = errorAt;
    }

    public static ConnectionRow read(LocalDb db, String ws) {
        Cursor c = db.getReadableDatabase().query("henu_connections", null,
                "workspace_id=?", new String[]{ws}, null, null, null);
        try {
            if (!c.moveToFirst()) return null;
            return new ConnectionRow(
                    c.getLong(c.getColumnIndexOrThrow("generation")),
                    c.getString(c.getColumnIndexOrThrow("state")),
                    str(c, "subject_fingerprint"),
                    c.getBlob(c.getColumnIndexOrThrow("session_ciphertext")),
                    str(c, "term_year"), str(c, "term_term"), str(c, "term_label"),
                    lng(c, "verified_at"),
                    str(c, "last_error_code"), str(c, "last_error_message"), lng(c, "last_error_at"));
        } finally {
            c.close();
        }
    }

    public static void upsertConnected(LocalDb db, String ws, long generation, byte[] ciphertext,
                                       String fingerprint, String termYear, String termTerm,
                                       String termLabel) {
        ContentValues v = new ContentValues();
        v.put("workspace_id", ws);
        v.put("school", "henu");
        v.put("generation", generation);
        v.put("state", "CONNECTED");
        v.put("subject_fingerprint", fingerprint);
        v.put("session_ciphertext", ciphertext);
        v.put("term_year", termYear);
        v.put("term_term", termTerm);
        v.put("term_label", termLabel);
        v.put("verified_at", System.currentTimeMillis());
        v.putNull("last_error_code");
        v.putNull("last_error_message");
        v.putNull("last_error_at");
        v.put("updated_at", System.currentTimeMillis());
        db.getWritableDatabase().replaceOrThrow("henu_connections", null, v);
    }

    public static void markVerified(LocalDb db, String ws, String termYear, String termTerm,
                                    String termLabel) {
        ContentValues v = new ContentValues();
        v.put("state", "CONNECTED");
        v.put("term_year", termYear);
        v.put("term_term", termTerm);
        v.put("term_label", termLabel);
        v.put("verified_at", System.currentTimeMillis());
        v.putNull("last_error_code");
        v.putNull("last_error_message");
        v.putNull("last_error_at");
        v.put("updated_at", System.currentTimeMillis());
        db.getWritableDatabase().update("henu_connections", v, "workspace_id=?", new String[]{ws});
    }

    /** Records a school-side error; null newState keeps the current state. */
    public static void markError(LocalDb db, String ws, String code, String message, String newState) {
        ContentValues v = new ContentValues();
        if (newState != null) v.put("state", newState);
        v.put("last_error_code", code);
        v.put("last_error_message", message);
        v.put("last_error_at", System.currentTimeMillis());
        v.put("updated_at", System.currentTimeMillis());
        db.getWritableDatabase().update("henu_connections", v, "workspace_id=?", new String[]{ws});
    }

    public static void disconnect(LocalDb db, String ws, long newGeneration) {
        ContentValues v = new ContentValues();
        v.put("generation", newGeneration);
        v.put("state", "DISCONNECTED");
        v.putNull("session_ciphertext");
        v.putNull("subject_fingerprint");
        v.putNull("term_year");
        v.putNull("term_term");
        v.putNull("term_label");
        v.putNull("verified_at");
        v.putNull("last_error_code");
        v.putNull("last_error_message");
        v.putNull("last_error_at");
        v.put("updated_at", System.currentTimeMillis());
        db.getWritableDatabase().update("henu_connections", v, "workspace_id=?", new String[]{ws});
    }

    private static String str(Cursor c, String col) {
        int i = c.getColumnIndex(col);
        return i < 0 || c.isNull(i) ? null : c.getString(i);
    }

    private static Long lng(Cursor c, String col) {
        int i = c.getColumnIndex(col);
        return i < 0 || c.isNull(i) ? null : c.getLong(i);
    }
}
