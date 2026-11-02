package com.xique.assistant;

import android.app.*;
import android.content.*;
import android.content.pm.PackageManager;
import android.os.Build;
import androidx.core.app.NotificationCompat;
import androidx.core.app.NotificationManagerCompat;
import org.json.*;
import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicBoolean;

/** One next-occurrence alarm; persisted preference, no polling/service/network. */
final class CourseReminders {
    static final String FIRE="com.xique.assistant.COURSE_REMINDER",OPEN="com.xique.assistant.OPEN_TIMETABLE",CHANNEL="course-reminders-v1";
    static final ExecutorService IO=Executors.newSingleThreadExecutor();
    private static final AtomicBoolean REBUILD_QUEUED=new AtomicBoolean();
    private static final int ALARM=8740;
    static SharedPreferences prefs(Context c){return c.getSharedPreferences("xique-course-reminders",Context.MODE_PRIVATE);}
    static boolean permitted(Context c){
        return (Build.VERSION.SDK_INT<33||c.checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS)==PackageManager.PERMISSION_GRANTED)&&NotificationManagerCompat.from(c).areNotificationsEnabled()&&(c.getSystemService(NotificationManager.class).getNotificationChannel(CHANNEL)==null||c.getSystemService(NotificationManager.class).getNotificationChannel(CHANNEL).getImportance()!=NotificationManager.IMPORTANCE_NONE);
    }
    static boolean exact(Context c){return Build.VERSION.SDK_INT<31||c.getSystemService(AlarmManager.class).canScheduleExactAlarms();}
    static void refreshAsync(Context c){if(!REBUILD_QUEUED.compareAndSet(false,true))return;Context app=c.getApplicationContext();IO.execute(()->{REBUILD_QUEUED.set(false);try{schedule(app);}catch(Exception ignored){prefs(app).edit().putBoolean("schedule_error",true).apply();}});}
    static JSONObject state(Context c)throws JSONException{
        SharedPreferences p=prefs(c);return new JSONObject().put("enabled",p.getBoolean("enabled",false)).put("minutes",p.getInt("minutes",10)).put("notification_permission",permitted(c)).put("exact_alarm",exact(c)).put("next_at",p.getLong("next_at",0)).put("schedule_error",p.getBoolean("schedule_error",false));
    }
    static List<CourseReminderPlanner.Course> courses(Context c)throws Exception{
        JSONArray rows=LocalCore.service(c).bootstrap().getJSONArray("courses");List<CourseReminderPlanner.Course> out=new ArrayList<>();
        for(int i=0;i<rows.length();i++){JSONObject r=rows.getJSONObject(i);out.add(new CourseReminderPlanner.Course(r.getLong("id"),r.getString("name"),r.optString("location"),r.getString("semester_start_date"),r.getString("start_time"),r.getString("week_pattern"),r.getInt("day_of_week")));}return out;
    }
    static PendingIntent alarm(Context c,String token){return PendingIntent.getBroadcast(c,ALARM,new Intent(c,CourseReminderReceiver.class).setAction(FIRE).putExtra("token",token),PendingIntent.FLAG_UPDATE_CURRENT|PendingIntent.FLAG_IMMUTABLE);}
    static void schedule(Context c)throws Exception{
        AlarmManager am=c.getSystemService(AlarmManager.class);SharedPreferences p=prefs(c);
        // Read a complete replacement before touching a previously valid alarm.
        // A transient database failure must not silently erase that alarm.
        List<CourseReminderPlanner.Event> plan=p.getBoolean("enabled",false)&&permitted(c)?CourseReminderPlanner.future(courses(c),System.currentTimeMillis(),p.getInt("minutes",10)):Collections.emptyList();
        am.cancel(alarm(c,""));
        p.edit().putLong("next_at",0).remove("token").putBoolean("schedule_error",false).apply();
        if(plan.isEmpty())return;
        CourseReminderPlanner.Event next=plan.get(0);String token=UUID.randomUUID().toString();
        // Commit the complete next-alarm identity before allowing delivery.
        if(!p.edit().putString("token",token).putLong("next_at",next.at).putLong("next_start",next.start).commit())throw new IllegalStateException("STORAGE_FAILURE");
        try{if(exact(c))am.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP,next.at,alarm(c,token));else am.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP,next.at,alarm(c,token));}
        catch(SecurityException revoked){am.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP,next.at,alarm(c,token));}
    }
    static void deliver(Context c,String token)throws Exception{
        SharedPreferences p=prefs(c);if(!p.getBoolean("enabled",false)||!permitted(c)||token==null||!token.equals(p.getString("token","")))return;
        long now=System.currentTimeMillis(),at=p.getLong("next_at",0),start=p.getLong("next_start",0);int minutes=p.getInt("minutes",10);
        // Regenerate from current SQLite rows so edits/deletions cannot send old details.
        List<CourseReminderPlanner.Event> due=CourseReminderPlanner.future(courses(c),at-1,minutes);
        for(CourseReminderPlanner.Event event:due){
            if(event.at!=at)break;if(event.start!=start||now>event.start+1000||now<at-1000)continue;
            String delivered="delivered:"+event.key;if(p.getBoolean(delivered,false))continue;
            NotificationManager nm=c.getSystemService(NotificationManager.class);nm.createNotificationChannel(new NotificationChannel(CHANNEL,"上课提醒",NotificationManager.IMPORTANCE_HIGH));
            String detail=event.course.start+" 上课 · "+(event.course.location.isEmpty()?"教室待确认":event.course.location);
            long remaining=Math.max(0,(event.start-now+59999)/60000);
            PendingIntent open=PendingIntent.getActivity(c,(int)(event.course.id%Integer.MAX_VALUE),new Intent(c,MainActivity.class).setAction(OPEN).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP|Intent.FLAG_ACTIVITY_CLEAR_TOP),PendingIntent.FLAG_UPDATE_CURRENT|PendingIntent.FLAG_IMMUTABLE);
            Notification notification=new NotificationCompat.Builder(c,CHANNEL).setSmallIcon(R.drawable.ic_stat_course).setContentTitle(event.course.name).setContentText(detail).setStyle(new NotificationCompat.BigTextStyle().bigText((remaining==0?"课程即将开始":"距离上课还有 "+remaining+" 分钟")+"\n"+detail)).setContentIntent(open).setAutoCancel(true).setCategory(NotificationCompat.CATEGORY_REMINDER).setVisibility(NotificationCompat.VISIBILITY_PRIVATE).build();
            nm.notify(event.key,(int)(event.course.id%Integer.MAX_VALUE),notification);p.edit().putBoolean(delivered,true).apply();
        }
        schedule(c);
    }
}
