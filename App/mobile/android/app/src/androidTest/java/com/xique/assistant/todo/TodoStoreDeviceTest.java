package com.xique.assistant.todo;

import android.content.*;
import android.database.sqlite.*;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.json.*;
import org.junit.*;
import org.junit.runner.RunWith;
import java.io.File;
import java.time.LocalDate;
import java.util.UUID;
import static org.junit.Assert.*;

@RunWith(AndroidJUnit4.class)
public class TodoStoreDeviceTest {
    TodoStore store;Context isolated;File file;
    @Before public void setup(){Context base=InstrumentationRegistry.getInstrumentation().getTargetContext();file=new File(base.getCacheDir(),"todo-test-"+UUID.randomUUID()+".db");isolated=new ContextWrapper(base){public File getDatabasePath(String name){return file;}};store=new TodoStore(isolated);}
    @After public void cleanup(){store.close();SQLiteDatabase.deleteDatabase(file);}
    private String id(){return UUID.randomUUID().toString();}
    private JSONObject item(String id)throws Exception{JSONArray a=store.state().getJSONArray("items");for(int i=0;i<a.length();i++)if(a.getJSONObject(i).getString("id").equals(id))return a.getJSONObject(i);return null;}
    @Test public void persistCreateEditAndIndependentDatabase()throws Exception{
        String id=id();store.create(id,"合成待办","第一行\n第二行",TodoStore.today(),true);JSONObject row=item(id);assertEquals(1,row.getLong("revision"));store.close();store=new TodoStore(isolated);assertEquals("第一行\n第二行",item(id).getString("note"));
        store.update(id,1,"更新后的合成待办","","",false);row=item(id);assertEquals(2,row.getLong("revision"));assertEquals("",row.getString("due_date"));assertFalse(row.getBoolean("important"));assertEquals(2,store.state().getLong("revision"));
    }
    @Test public void replayAndNoopDoNotDuplicateOrBump()throws Exception{
        String id=id();store.create(id,"合成待办","","",false);store.create(id.toUpperCase(),"合成待办","","",false);store.update(id,1,"合成待办","","",false);store.complete(id,1,false);assertEquals(1,store.state().getLong("revision"));assertEquals(1,store.state().getJSONArray("items").length());
        try{store.create(id,"不同内容","","",false);fail();}catch(IllegalArgumentException e){assertEquals("REVISION_CONFLICT",e.getMessage());}
    }
    @Test public void staleWidgetCannotCompleteEditedTaskOrReopenCompletedTask()throws Exception{
        String id=id();store.create(id,"合成待办","","",false);store.update(id,1,"修改后的合成待办","","",true);
        try{store.complete(id,1,true);fail();}catch(IllegalArgumentException e){assertEquals("REVISION_CONFLICT",e.getMessage());}assertFalse(item(id).getBoolean("completed"));
        store.complete(id,2,true);long revision=store.state().getLong("revision");store.complete(id,2,true);assertEquals(revision,store.state().getLong("revision"));assertTrue(item(id).getBoolean("completed"));
        store.complete(id,3,false);try{store.complete(id,2,true);fail();}catch(IllegalArgumentException expected){}assertFalse(item(id).getBoolean("completed"));
    }
    @Test public void mutationFailureRollsBackWholeTransaction()throws Exception{
        store.state();store.getWritableDatabase().execSQL("CREATE TRIGGER qa_todo_failure BEFORE UPDATE ON todo_meta BEGIN SELECT RAISE(ABORT,'synthetic failure'); END");String id=id();
        try{store.create(id,"合成待办","","",false);fail();}catch(SQLiteException expected){}assertEquals(0,store.state().getJSONArray("items").length());assertEquals(0,store.state().getLong("revision"));store.getWritableDatabase().execSQL("DROP TRIGGER qa_todo_failure");store.create(id,"合成待办","","",false);assertEquals(1,store.state().getLong("revision"));
    }
    @Test public void dateGroupsCountsCompletionAndDelete()throws Exception{
        String overdue=id(),today=id(),important=id(),later=id(),plain=id();String day=TodoStore.today();store.create(plain,"普通","","",false);store.create(later,"未来","",LocalDate.parse(day).plusDays(2).toString(),false);store.create(important,"重要","","",true);store.create(today,"今天","",day,false);store.create(overdue,"逾期","",LocalDate.parse(day).minusDays(1).toString(),false);
        JSONArray rows=store.state().getJSONArray("items");assertEquals(overdue,rows.getJSONObject(0).getString("id"));assertEquals(today,rows.getJSONObject(1).getString("id"));assertEquals(important,rows.getJSONObject(2).getString("id"));assertEquals(later,rows.getJSONObject(3).getString("id"));assertEquals(plain,rows.getJSONObject(4).getString("id"));
        assertEquals(1,store.state().getJSONObject("counts").getInt("today"));store.complete(today,1,true);assertEquals(0,store.state().getJSONObject("counts").getInt("today"));assertEquals(1,store.state().getJSONObject("counts").getInt("completed"));
        try{store.remove(today,1);fail();}catch(IllegalArgumentException expected){}store.remove(today,2);long revision=store.state().getLong("revision");store.remove(today,2);assertEquals(revision,store.state().getLong("revision"));assertNull(item(today));
    }
    @Test public void rejectsInvalidContentAndDates()throws Exception{
        for(String title:new String[]{" ","x\ny",new String(new char[121]).replace('\0','字')})try{store.create(id(),title,"","",false);fail();}catch(IllegalArgumentException expected){}
        for(String date:new String[]{"2026-02-30","26-01-01","2101-01-01","2026-13-01"})try{store.create(id(),"内容","",date,false);fail();}catch(IllegalArgumentException expected){}
        try{store.create("bad","内容","","",false);fail();}catch(IllegalArgumentException expected){}
        try{store.create(id(),"内容",new String(new char[2001]).replace('\0','a'),"",false);fail();}catch(IllegalArgumentException expected){}assertEquals(0,store.state().getJSONArray("items").length());
    }
    @Test public void twoRepositoryConnectionsDoNotLoseUpdates()throws Exception{
        String id=id();store.create(id,"内容","","",false);try(TodoStore other=new TodoStore(isolated)){other.update(id,1,"新内容","","",false);try{store.update(id,1,"旧编辑","","",false);fail();}catch(IllegalArgumentException expected){}assertEquals("新内容",item(id).getString("title"));assertEquals(2,item(id).getLong("revision"));}
    }
}
