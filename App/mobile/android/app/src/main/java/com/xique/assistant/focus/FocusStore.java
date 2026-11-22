package com.xique.assistant.focus;

import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;
import android.database.sqlite.SQLiteOpenHelper;
import org.json.JSONArray;
import org.json.JSONObject;

/** Independent of the school database. Snapshot and completion are one atomic transaction. */
final class FocusStore extends SQLiteOpenHelper {
    FocusStore(Context context) { super(context,context.getDatabasePath("focus.db").getAbsolutePath(),null,1); }
    @Override public void onCreate(SQLiteDatabase db) {
        db.execSQL("CREATE TABLE state (id INTEGER PRIMARY KEY CHECK(id=1), payload TEXT NOT NULL)");
        db.execSQL("CREATE TABLE records (id TEXT PRIMARY KEY, started_at INTEGER NOT NULL, actual_ms INTEGER NOT NULL, mode TEXT NOT NULL, completed INTEGER NOT NULL, payload TEXT NOT NULL)");
        db.execSQL("CREATE INDEX records_started ON records(started_at)");
    }
    @Override public void onUpgrade(SQLiteDatabase db,int oldVersion,int newVersion) { throw new IllegalStateException("Unsupported focus schema"); }
    JSONObject read() throws Exception {
        try(Cursor c=getReadableDatabase().rawQuery("SELECT payload FROM state WHERE id=1",null)) {
            return c.moveToFirst()?new JSONObject(c.getString(0)):null;
        }
    }
    boolean recorded(String id) {
        try(Cursor c=getReadableDatabase().rawQuery("SELECT 1 FROM records WHERE id=?",new String[]{id})) { return c.moveToFirst(); }
    }
    void write(JSONObject state,JSONObject record) throws Exception {
        SQLiteDatabase db=getWritableDatabase(); db.beginTransaction();
        try {
            if(record!=null) {
                db.insertOrThrow("records", null, recordValues(record));
            }
            writeState(db, state);
            db.setTransactionSuccessful();
        } finally { db.endTransaction(); }
    }
    private static ContentValues recordValues(JSONObject record) throws Exception {
        ContentValues values = new ContentValues();
        values.put("id", record.getString("id"));
        values.put("started_at", record.getLong("started_at"));
        values.put("actual_ms", record.getLong("actual_ms"));
        values.put("mode", record.getString("mode"));
        values.put("completed", record.getBoolean("completed") ? 1 : 0);
        values.put("payload", record.toString());
        return values;
    }

    private static void writeState(SQLiteDatabase db, JSONObject state) {
        ContentValues values = new ContentValues();
        values.put("id", 1);
        values.put("payload", state.toString());
        db.replaceOrThrow("state", null, values);
    }

    JSONArray backupRecords(int maximum) throws Exception {
        JSONArray out = new JSONArray();
        try (Cursor rows = getReadableDatabase().rawQuery("SELECT payload FROM records ORDER BY started_at,id LIMIT ?",
                new String[]{String.valueOf(maximum + 1)})) {
            while (rows.moveToNext()) {
                if (out.length() >= maximum) throw new IllegalArgumentException("BACKUP_TOO_LARGE");
                out.put(new JSONObject(rows.getString(0)));
            }
        }
        return out;
    }

    int mergeBackup(JSONArray records, JSONObject state) throws Exception {
        SQLiteDatabase db = getWritableDatabase();
        db.beginTransaction();
        try {
            JSONObject timer = state.optJSONObject("timer");
            String activeId = timer == null ? "" : timer.getString("id");
            int inserted = 0;
            for (int i = 0; i < records.length(); i++) {
                JSONObject record = records.getJSONObject(i);
                String id = record.getString("id");
                if (id.equals(activeId) || recorded(id)) continue;
                db.insertOrThrow("records", null, recordValues(record));
                inserted++;
            }
            if (inserted > 0) {
                state.put("revision", state.getLong("revision") + 1);
                writeState(db, state);
            }
            db.setTransactionSuccessful();
            return inserted;
        } finally {
            db.endTransaction();
        }
    }
    JSONArray recent() throws Exception {
        JSONArray out=new JSONArray();
        try(Cursor c=getReadableDatabase().rawQuery("SELECT payload FROM records ORDER BY started_at DESC,id DESC LIMIT 100",null)) { while(c.moveToNext()) out.put(new JSONObject(c.getString(0))); }return out;
    }
    JSONObject totals(long start,long end) throws Exception {
        try(Cursor c=getReadableDatabase().rawQuery("SELECT COALESCE(SUM(actual_ms),0), COALESCE(SUM(CASE WHEN mode='focus' AND completed=1 THEN 1 ELSE 0 END),0) FROM records WHERE mode IN ('focus','countup') AND started_at>=? AND started_at<?",new String[]{""+start,""+end})) {
            c.moveToFirst();return new JSONObject().put("seconds",c.getLong(0)/1000).put("pomodoros",c.getLong(1));
        }
    }
    JSONArray daily(long start,long end) throws Exception {
        JSONArray out=new JSONArray();
        try(Cursor c=getReadableDatabase().rawQuery("SELECT date(started_at/1000,'unixepoch','+8 hours'),COALESCE(SUM(actual_ms),0)/1000,COALESCE(SUM(CASE WHEN mode='focus' AND completed=1 THEN 1 ELSE 0 END),0) FROM records WHERE mode IN ('focus','countup') AND started_at>=? AND started_at<? GROUP BY 1 ORDER BY 1",new String[]{""+start,""+end})) {
            while(c.moveToNext())out.put(new JSONObject().put("date",c.getString(0)).put("seconds",c.getLong(1)).put("pomodoros",c.getLong(2)));
        }return out;
    }
}
