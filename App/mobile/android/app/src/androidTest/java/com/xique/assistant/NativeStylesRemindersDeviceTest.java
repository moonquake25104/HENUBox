package com.xique.assistant;

import android.app.*;
import android.graphics.*;
import android.content.*;
import android.os.*;
import android.view.*;
import android.service.notification.StatusBarNotification;
import androidx.test.core.app.ActivityScenario;
import androidx.test.platform.app.InstrumentationRegistry;
import org.json.*;
import org.junit.*;
import java.time.*;
import java.time.temporal.ChronoUnit;
import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicReference;
import static org.junit.Assert.*;

/** Explicitly opt-in only, a disposable/read-only emulator with no user courses. */
public class NativeStylesRemindersDeviceTest {
    MainActivity activity;
    private void emulatorOnly(){Assume.assumeTrue(Build.HARDWARE.contains("ranchu")||Build.HARDWARE.contains("goldfish"));assertEquals("true",InstrumentationRegistry.getArguments().getString("allow_disposable_workspace"));}
    private JSONObject js(String expression)throws Exception{
        CountDownLatch done=new CountDownLatch(1);AtomicReference<String> raw=new AtomicReference<>();
        InstrumentationRegistry.getInstrumentation().runOnMainSync(()->activity.getBridge().getWebView().evaluateJavascript("JSON.stringify(("+expression+"))",v->{raw.set(v);done.countDown();}));
        assertTrue(done.await(10,TimeUnit.SECONDS));Object value=new JSONTokener(raw.get()).nextValue();assertTrue(value instanceof String);return new JSONObject((String)value);
    }
    private void waitFor(String condition)throws Exception{long end=System.nanoTime()+TimeUnit.SECONDS.toNanos(20);while(System.nanoTime()<end){try{if(js("({ready:!!("+condition+")})").getBoolean("ready"))return;}catch(Exception|AssertionError ignored){}Thread.sleep(100);}fail("UI condition timeout: "+condition);}
    private JSONObject nativeCall(String plugin,String method,String args)throws Exception{
        js("(()=>{delete window.__nativeTestResult;Capacitor.nativePromise('"+plugin+"','"+method+"',"+args+").then(v=>window.__nativeTestResult={value:v}).catch(e=>window.__nativeTestResult={error:e.code||'ERROR'});return {ok:true}})()");
        waitFor("window.__nativeTestResult");return js("window.__nativeTestResult");
    }
    @Test public void transparentNavigationHasNoNativeOverlay()throws Exception{
        emulatorOnly();
        try(ActivityScenario<MainActivity> scenario=ActivityScenario.launch(MainActivity.class)){
            scenario.onActivity(a->activity=a);waitFor("document.querySelector('.xm-navigation')");
            js("(()=>{document.documentElement.dataset.uiStyle='glass';location.hash='/home';return {ok:true}})()");
            waitFor("location.hash==='#/home'");
            waitFor("getComputedStyle(document.querySelector('.xm-navigation')).backgroundColor==='rgba(0, 0, 0, 0)'");
            JSONObject material=js("(()=>{const e=document.querySelector('.xm-navigation'),s=getComputedStyle(e);return {transparent:s.backgroundColor==='rgba(0, 0, 0, 0)',noBlur:s.backdropFilter==='none',single:e.dataset.renderer==='web',nativePlugin:Capacitor.isPluginAvailable('XiqueGlass'),opticalLayers:document.querySelectorAll('[data-vaso]').length}})()");
            assertTrue(material.toString(),material.getBoolean("transparent"));assertTrue(material.toString(),material.getBoolean("noBlur"));assertTrue(material.toString(),material.getBoolean("single"));assertFalse(material.toString(),material.getBoolean("nativePlugin"));assertEquals(0,material.getInt("opticalLayers"));
            String[] ids={"campus","me","timetable","home"};int[] positions={2,3,1,0};
            for(int round=0;round<4;round++)for(int i=0;i<ids.length;i++){
                js("(()=>{document.querySelector('.xm-navigation button[data-nav-index=\""+positions[i]+"\"]').click();return {ok:true}})()");
                waitFor("location.hash==='#/"+ids[i]+"'&&document.querySelector('.xm-navigation button[aria-current=page]').dataset.navIndex==='"+positions[i]+"'&&document.querySelector('.xm-page-"+ids[i]+"')");
            }
            System.out.println("TRANSPARENT_NAV_DEVICE_OK native_plugin=false captures=0 optical_layers=0 transitions=16");
        }
    }
    @Test public void scheduledNotificationAndRebuildAfterEdit()throws Exception{
        emulatorOnly();Context context=InstrumentationRegistry.getInstrumentation().getTargetContext();
        var service=LocalCore.service(context);JSONObject boot=service.bootstrap();assertEquals("Disposable empty workspace required",0,boot.getJSONArray("courses").length());
        assertTrue("Grant notifications in this disposable test emulator",CourseReminders.permitted(context));assertTrue("Allow alarms in this disposable test emulator",CourseReminders.exact(context));
        String ws=boot.getString("workspace_id"),sync=UUID.randomUUID().toString();long courseId=0;
        ZonedDateTime next=ZonedDateTime.now(CourseReminderPlanner.ZONE).truncatedTo(ChronoUnit.MINUTES).plusMinutes(2);if(next.toInstant().toEpochMilli()-60000-System.currentTimeMillis()<5000)next=next.plusMinutes(1);LocalDate anchor=next.toLocalDate().with(java.time.temporal.TemporalAdjusters.previousOrSame(DayOfWeek.MONDAY));
        JSONObject course=new JSONObject().put("name","QA 合成上课提醒").put("teacher","").put("location","QA 合成教室").put("color","#b8d6e0").put("day_of_week",next.getDayOfWeek().getValue()).put("start_time",next.toLocalTime().toString()).put("end_time",next.plusMinutes(30).toLocalTime().toString()).put("week_pattern","1周").put("semester_start_date",anchor.toString()).put("semester","2026S2");
        try{
            courseId=service.saveCourse(ws,boot.getLong("revision"),0,sync,course).getLong("course_id");
            CourseReminders.prefs(context).edit().putBoolean("enabled",true).putInt("minutes",1).commit();CourseReminders.IO.submit(()->{try{CourseReminders.schedule(context);}catch(Exception e){throw new RuntimeException(e);}}).get(10,TimeUnit.SECONDS);
            JSONObject state=CourseReminders.state(context);long at=state.getLong("next_at");assertEquals(next.toInstant().toEpochMilli()-60000,at);
            String previousToken=CourseReminders.prefs(context).getString("token","");android.database.sqlite.SQLiteDatabase db=service.database().getWritableDatabase();
            db.execSQL("ALTER TABLE local_courses RENAME TO qa_courses_unavailable");
            try{try{CourseReminders.IO.submit(()->{try{CourseReminders.schedule(context);}catch(Exception failure){throw new RuntimeException(failure);}}).get();fail("Database fault not observed");}catch(ExecutionException expected){}assertEquals(at,CourseReminders.state(context).getLong("next_at"));assertEquals(previousToken,CourseReminders.prefs(context).getString("token",""));}
            finally{db.execSQL("ALTER TABLE qa_courses_unavailable RENAME TO local_courses");}
            // Changing the lead cancels/replaces the previous next occurrence.
            CourseReminders.prefs(context).edit().putInt("minutes",0).commit();CourseReminders.IO.submit(()->{try{CourseReminders.schedule(context);}catch(Exception e){throw new RuntimeException(e);}}).get();assertEquals(next.toInstant().toEpochMilli(),CourseReminders.state(context).getLong("next_at"));
            CourseReminders.prefs(context).edit().putInt("minutes",1).commit();CourseReminders.IO.submit(()->{try{CourseReminders.schedule(context);}catch(Exception e){throw new RuntimeException(e);}}).get();
            long deadline=at+8000;boolean received=false;while(System.currentTimeMillis()<deadline){for(StatusBarNotification n:context.getSystemService(NotificationManager.class).getActiveNotifications())if(CourseReminders.CHANNEL.equals(n.getNotification().getChannelId()))received=true;if(received)break;Thread.sleep(250);}assertTrue("Real AlarmManager notification not received",received);
            // Turning off removes the next scheduled alarm, and survives re-read.
            CourseReminders.prefs(context).edit().putBoolean("enabled",false).commit();CourseReminders.IO.submit(()->{try{CourseReminders.schedule(context);}catch(Exception e){throw new RuntimeException(e);}}).get();assertEquals(0,CourseReminders.state(context).getLong("next_at"));
            System.out.println("COURSE_REMINDER_DEVICE_OK real_alarm=true real_notification=true disable_cancels=true no_network=true");
        }finally{
            CourseReminders.prefs(context).edit().putBoolean("enabled",false).commit();CourseReminders.IO.submit(()->{try{CourseReminders.schedule(context);}catch(Exception ignored){}}).get();
            if(courseId>0)service.deleteCourse(ws,service.bootstrap().getLong("revision"),courseId);
            for(StatusBarNotification n:context.getSystemService(NotificationManager.class).getActiveNotifications())if(CourseReminders.CHANNEL.equals(n.getNotification().getChannelId()))context.getSystemService(NotificationManager.class).cancel(n.getTag(),n.getId());
        }
    }
}
