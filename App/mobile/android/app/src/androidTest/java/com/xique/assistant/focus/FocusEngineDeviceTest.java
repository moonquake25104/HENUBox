package com.xique.assistant.focus;

import android.content.Context;
import android.content.ContextWrapper;
import androidx.test.platform.app.InstrumentationRegistry;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import org.junit.*;
import org.junit.runner.RunWith;
import org.json.*;
import java.io.File;
import java.util.UUID;
import static org.junit.Assert.*;

/** Synthetic clock and isolated database; never touches personal courses, account or focus DB. */
@RunWith(AndroidJUnit4.class)
public class FocusEngineDeviceTest {
    static class Time implements FocusEngine.Clock {long mono=10000,wall=1793376000000L;int boot=4;public long elapsed(){return mono;}public long wall(){return wall;}public int boot(){return boot;}void advance(long ms){mono+=ms;wall+=ms;}}
    Time time;FocusEngine engine;Context isolated;File db;
    @Before public void setup(){
        Context base=InstrumentationRegistry.getInstrumentation().getTargetContext();db=new File(base.getCacheDir(),"focus-test-"+UUID.randomUUID()+".db");
        isolated=new ContextWrapper(base){@Override public File getDatabasePath(String name){return db;}};
        time=new Time();engine=new FocusEngine(isolated,time);
    }
    private JSONObject state()throws Exception{return engine.getState();}
    private JSONObject command(String action,String mode,String label)throws Exception{return engine.command(action,state().getLong("revision"),UUID.randomUUID().toString(),mode,label);}
    private void minuteSettings(boolean auto)throws Exception{JSONObject s=state(),p=s.getJSONObject("settings");p.put("focus_minutes",1).put("short_minutes",1).put("long_minutes",1).put("long_every",2).put("auto_break",auto);engine.saveSettings(s.getLong("revision"),p);}
    @After public void cleanup(){engine.close();android.database.sqlite.SQLiteDatabase.deleteDatabase(db);}
    @Test public void pauseWallChangeAndColdRestore()throws Exception{
        command("start","focus","合成标签");time.advance(10000);time.wall+=999999999;
        assertEquals(10000,state().getJSONObject("timer").getLong("elapsed_ms"));
        command("pause","","");time.advance(80000);assertEquals(10000,state().getJSONObject("timer").getLong("elapsed_ms"));
        engine.close();engine=new FocusEngine(isolated,time);assertEquals("PAUSED",state().getJSONObject("timer").getString("status"));
        command("resume","","");time.advance(5000);assertEquals(15000,state().getJSONObject("timer").getLong("elapsed_ms"));
        command("finish","","");assertEquals(15000,state().getJSONArray("records").getJSONObject(0).getLong("actual_ms"));assertEquals(0,state().getJSONObject("today").getInt("pomodoros"));
    }
    @Test public void naturalFinishExactlyOnceAndRestartDoesNotInventRounds()throws Exception{
        minuteSettings(true);command("start","focus","");time.advance(3600000);engine.close();engine=new FocusEngine(isolated,time);
        JSONObject s=state();assertTrue(s.isNull("timer"));assertEquals(1,s.getJSONArray("records").length());assertEquals(60,s.getJSONObject("today").getInt("seconds"));assertEquals(1,s.getJSONObject("today").getInt("pomodoros"));
        assertEquals(1,state().getJSONArray("records").length());
    }
    @Test public void activeAutoBreakAndBreakExcludedFromStats()throws Exception{
        minuteSettings(true);command("start","focus","阅读");engine.setForegroundAttached(true);time.advance(60000);JSONObject s=engine.tick();assertEquals("short_break",s.getJSONObject("timer").getString("mode"));
        time.advance(60000);s=engine.tick();assertTrue(s.isNull("timer"));assertEquals(2,s.getJSONArray("records").length());assertEquals(60,s.getJSONObject("today").getInt("seconds"));assertEquals(1,s.getJSONObject("today").getInt("pomodoros"));
    }
    @Test public void rebootInterruptAndCheckpoint()throws Exception{
        command("start","countup","");time.advance(20000);engine.tick();time.advance(8000);time.boot++;time.mono=100;
        engine.close();engine=new FocusEngine(isolated,time);JSONObject t=state().getJSONObject("timer");assertEquals("INTERRUPTED",t.getString("status"));assertEquals(20000,t.getLong("elapsed_ms"));
        command("resume","","");time.advance(3000);command("finish","","");assertEquals(23000,state().getJSONArray("records").getJSONObject(0).getLong("actual_ms"));
    }
    @Test public void revisionsAndIdentityAndStrictSettings()throws Exception{
        JSONObject s=state();String id=UUID.randomUUID().toString();engine.command("start",0,id,"focus","测试");
        assertEquals(1,engine.command("start",0,id,"focus","测试").getLong("revision"));
        try{engine.command("start",0,id,"countup","测试");fail();}catch(IllegalArgumentException expected){}
        try{engine.command("pause",0,UUID.randomUUID().toString(),"","");fail();}catch(IllegalArgumentException expected){assertEquals("REVISION_CONFLICT",expected.getMessage());}
        engine.notificationAction("finish",UUID.randomUUID().toString());assertNotNull(state().optJSONObject("timer"));
        JSONObject p=s.getJSONObject("settings");p.put("focus_minutes",1.5);try{FocusEngine.validateSettings(p);fail();}catch(IllegalArgumentException expected){}
    }
    @Test public void tagsArchiveKeepsHistoryAndCountupNotPomodoro()throws Exception{
        command("tag","","读书");command("start","countup","读书");time.advance(30000);command("finish","","");command("archive","","读书");
        JSONObject s=state();assertEquals("读书",s.getJSONArray("records").getJSONObject(0).getString("label"));assertEquals(30,s.getJSONObject("today").getInt("seconds"));assertEquals(0,s.getJSONObject("today").getInt("pomodoros"));assertFalse(s.getJSONArray("labels").toString().contains("读书"));
        long revision=s.getLong("revision");try{engine.command("finish",revision,UUID.randomUUID().toString(),"","");fail();}catch(IllegalArgumentException expected){}assertEquals(revision,state().getLong("revision"));assertEquals(1,state().getJSONArray("records").length());
    }
    @Test public void heatmapIsBoundedAndExcludesBreaks()throws Exception{
        command("start","countup","");time.advance(90000);command("finish","","");command("start","short_break","");time.advance(10000);command("finish","","");
        JSONObject h=state().getJSONObject("heatmap");assertEquals(84,h.getJSONArray("days").length());assertEquals(90,h.getInt("total_seconds"));
        long sum=0;for(int i=0;i<84;i++)sum+=h.getJSONArray("days").getJSONObject(i).getInt("seconds");assertEquals(90,sum);
    }
}
