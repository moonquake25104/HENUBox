package com.xique.assistant.todo;

import android.content.*;
import android.database.Cursor;
import android.database.sqlite.*;
import org.json.*;
import java.time.*;
import java.util.*;

/** One local repository shared by the WebView bridge and launcher widget. */
public final class TodoStore extends SQLiteOpenHelper {
    private static TodoStore instance;
    public static synchronized TodoStore get(Context c){if(instance==null)instance=new TodoStore(c.getApplicationContext());return instance;}
    public TodoStore(Context c){super(c,c.getDatabasePath("todos.db").getAbsolutePath(),null,1);}
    @Override public void onCreate(SQLiteDatabase db){
        db.execSQL("CREATE TABLE todo_meta(id INTEGER PRIMARY KEY CHECK(id=1),revision INTEGER NOT NULL)");db.execSQL("INSERT INTO todo_meta VALUES(1,0)");
        db.execSQL("CREATE TABLE todos(id TEXT PRIMARY KEY,title TEXT NOT NULL,note TEXT NOT NULL,due_date TEXT NOT NULL,important INTEGER NOT NULL,completed INTEGER NOT NULL,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL,completed_at INTEGER,revision INTEGER NOT NULL)");
        db.execSQL("CREATE INDEX todo_open_dates ON todos(completed,due_date)");
    }
    @Override public void onUpgrade(SQLiteDatabase db,int oldVersion,int newVersion){throw new IllegalStateException("Unsupported todo schema");}
    public static String today(){return LocalDate.now(ZoneId.of("Asia/Shanghai")).toString();}
    private static String id(String value){if(value==null||!value.matches("[a-fA-F0-9]{8}(-[a-fA-F0-9]{4}){3}-[a-fA-F0-9]{12}"))throw new IllegalArgumentException("INVALID_ARGUMENT");return value.toLowerCase(Locale.ROOT);}
    private static String title(String value){if(value==null)throw new IllegalArgumentException("INVALID_ARGUMENT");String s=value.trim();if(s.isEmpty()||s.codePointCount(0,s.length())>120||s.chars().anyMatch(Character::isISOControl))throw new IllegalArgumentException("INVALID_ARGUMENT");return s;}
    private static void validate(String note,String date){
        if(note==null||note.codePointCount(0,note.length())>2000||note.indexOf('\0')>=0||date==null)throw new IllegalArgumentException("INVALID_ARGUMENT");
        if(!date.isEmpty())try{if(!date.matches("\\d{4}-\\d{2}-\\d{2}")||!LocalDate.parse(date).toString().equals(date)||date.compareTo("2000-01-01")<0||date.compareTo("2100-12-31")>0)throw new Exception();}catch(Exception e){throw new IllegalArgumentException("INVALID_ARGUMENT");}
    }
    private JSONObject row(Cursor c)throws JSONException{return new JSONObject().put("id",c.getString(0)).put("title",c.getString(1)).put("note",c.getString(2)).put("due_date",c.getString(3)).put("important",c.getInt(4)!=0).put("completed",c.getInt(5)!=0).put("created_at",c.getLong(6)).put("updated_at",c.getLong(7)).put("completed_at",c.isNull(8)?JSONObject.NULL:c.getLong(8)).put("revision",c.getLong(9));}
    private JSONObject find(SQLiteDatabase db,String id)throws JSONException{try(Cursor c=db.rawQuery("SELECT * FROM todos WHERE id=?",new String[]{id})){return c.moveToFirst()?row(c):null;}}
    private void bump(SQLiteDatabase db){db.execSQL("UPDATE todo_meta SET revision=revision+1 WHERE id=1");}
    private void verify(JSONObject item,long revision){if(item==null)throw new IllegalArgumentException("NOT_FOUND");if(revision<0||revision>9007199254740991L||item.optLong("revision")!=revision)throw new IllegalArgumentException("REVISION_CONFLICT");}
    private boolean same(JSONObject item,String title,String note,String date,boolean important){return item!=null&&item.optString("title").equals(title)&&item.optString("note").equals(note)&&item.optString("due_date").equals(date)&&item.optBoolean("important")==important;}
    private ContentValues content(String title,String note,String date,boolean important){ContentValues v=new ContentValues();v.put("title",title);v.put("note",note);v.put("due_date",date);v.put("important",important?1:0);v.put("updated_at",System.currentTimeMillis());return v;}
    public synchronized JSONObject state()throws JSONException{
        SQLiteDatabase db=getReadableDatabase();db.beginTransaction();try{
            long revision;try(Cursor c=db.rawQuery("SELECT revision FROM todo_meta WHERE id=1",null)){if(!c.moveToFirst())throw new SQLiteException("Missing todo metadata");revision=c.getLong(0);}
            String day=today();JSONArray items=new JSONArray();int open=0,completed=0,dueToday=0,overdue=0;
            String order="SELECT * FROM todos ORDER BY completed ASC,CASE WHEN completed=1 THEN 0 WHEN due_date<>'' AND due_date<? THEN 0 WHEN due_date=? THEN 1 WHEN important=1 THEN 2 WHEN due_date<>'' THEN 3 ELSE 4 END ASC,CASE WHEN completed=0 THEN due_date END ASC,CASE WHEN completed=1 THEN completed_at END DESC,created_at ASC,id ASC";
            try(Cursor c=db.rawQuery(order,new String[]{day,day})){while(c.moveToNext()){JSONObject item=row(c);items.put(item);if(item.getBoolean("completed"))completed++;else{open++;String date=item.getString("due_date");if(date.equals(day))dueToday++;if(!date.isEmpty()&&date.compareTo(day)<0)overdue++;}}}
            JSONObject result=new JSONObject().put("schema_version",1).put("revision",revision).put("today",day).put("items",items).put("counts",new JSONObject().put("open",open).put("completed",completed).put("today",dueToday).put("overdue",overdue));db.setTransactionSuccessful();return result;
        }finally{db.endTransaction();}
    }
    public synchronized JSONObject create(String rawId,String rawTitle,String note,String date,boolean important)throws JSONException{
        String id=id(rawId),title=title(rawTitle);validate(note,date);SQLiteDatabase db=getWritableDatabase();db.beginTransaction();try{
            JSONObject old=find(db,id);if(old!=null){if(!same(old,title,note,date,important))throw new IllegalArgumentException("REVISION_CONFLICT");}
            else{try(Cursor c=db.rawQuery("SELECT count(*) FROM todos",null)){c.moveToFirst();if(c.getInt(0)>=500)throw new IllegalArgumentException("TODO_LIMIT");}
                ContentValues v=content(title,note,date,important);v.put("id",id);v.put("created_at",System.currentTimeMillis());v.put("completed",0);v.putNull("completed_at");v.put("revision",1);db.insertOrThrow("todos",null,v);bump(db);}
            db.setTransactionSuccessful();
        }finally{db.endTransaction();}return state();
    }
    public synchronized JSONObject update(String rawId,long revision,String rawTitle,String note,String date,boolean important)throws JSONException{
        String id=id(rawId),title=title(rawTitle);validate(note,date);SQLiteDatabase db=getWritableDatabase();db.beginTransaction();try{
            JSONObject old=find(db,id);verify(old,revision);if(!same(old,title,note,date,important)){ContentValues v=content(title,note,date,important);v.put("revision",revision+1);db.update("todos",v,"id=?",new String[]{id});bump(db);}db.setTransactionSuccessful();
        }finally{db.endTransaction();}return state();
    }
    public synchronized JSONObject complete(String rawId,long revision,boolean completed)throws JSONException{
        String id=id(rawId);SQLiteDatabase db=getWritableDatabase();db.beginTransaction();try{
            JSONObject old=find(db,id);if(old==null)throw new IllegalArgumentException("NOT_FOUND");
            if(old.getBoolean("completed")!=completed){verify(old,revision);ContentValues v=new ContentValues();v.put("completed",completed?1:0);v.put("updated_at",System.currentTimeMillis());if(completed)v.put("completed_at",System.currentTimeMillis());else v.putNull("completed_at");v.put("revision",revision+1);db.update("todos",v,"id=?",new String[]{id});bump(db);}db.setTransactionSuccessful();
        }finally{db.endTransaction();}return state();
    }
    public synchronized JSONObject remove(String rawId,long revision)throws JSONException{
        String id=id(rawId);SQLiteDatabase db=getWritableDatabase();db.beginTransaction();try{JSONObject old=find(db,id);if(old!=null){verify(old,revision);db.delete("todos","id=?",new String[]{id});bump(db);}db.setTransactionSuccessful();}finally{db.endTransaction();}return state();
    }
}
