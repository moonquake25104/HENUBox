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
    private static final long DELIVERY_GRACE_MS = 2 * 60_000L;
    static SharedPreferences prefs(Context c){return c.getSharedPreferences("xique-course-reminders",Context.MODE_PRIVATE);}
    static boolean permitted(Context c){
        return (Build.VERSION.SDK_INT<33||c.checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS)==PackageManager.PERMISSION_GRANTED)&&NotificationManagerCompat.from(c).areNotificationsEnabled()&&(c.getSystemService(NotificationManager.class).getNotificationChannel(CHANNEL)==null||c.getSystemService(NotificationManager.class).getNotificationChannel(CHANNEL).getImportance()!=NotificationManager.IMPORTANCE_NONE);
    }
    static boolean exact(Context c){return Build.VERSION.SDK_INT<31||c.getSystemService(AlarmManager.class).canScheduleExactAlarms();}
    static void refreshAsync(Context c){if(!REBUILD_QUEUED.compareAndSet(false,true))return;Context app=c.getApplicationContext();TimetableWidget.refresh(app);WeekTimetableWidget.refresh(app);IO.execute(()->{REBUILD_QUEUED.set(false);try{schedule(app);}catch(Exception ignored){prefs(app).edit().putBoolean("schedule_error",true).apply();}});}
    static JSONObject state(Context c)throws JSONException{
        SharedPreferences p=prefs(c);return new JSONObject().put("enabled",p.getBoolean("enabled",false)).put("minutes",p.getInt("minutes",10)).put("notification_permission",permitted(c)).put("exact_alarm",exact(c)).put("next_at",p.getLong("next_at",0)).put("schedule_error",p.getBoolean("schedule_error",false));
    }
    static List<CourseReminderPlanner.Course> courses(Context c)throws Exception{
        JSONArray rows=LocalCore.service(c).bootstrap().getJSONArray("courses");List<CourseReminderPlanner.Course> out=new ArrayList<>();
        for(int i=0;i<rows.length();i++){JSONObject r=rows.getJSONObject(i);out.add(new CourseReminderPlanner.Course(r.getLong("id"),r.getString("name"),r.optString("location"),r.getString("semester_start_date"),r.getString("start_time"),r.getString("week_pattern"),r.getInt("day_of_week")));}return out;
    }
    private static PendingIntent alarm(Context c, String token, int flags) {
        Intent intent = new Intent(c, CourseReminderReceiver.class).setAction(FIRE).putExtra("token", token);
        return PendingIntent.getBroadcast(c, ALARM, intent, flags | PendingIntent.FLAG_IMMUTABLE);
    }

    private static String deliveredKey(CourseReminderPlanner.Event event) {
        return "delivered:" + event.key;
    }

    static void schedule(Context c)throws Exception{
        AlarmManager am=c.getSystemService(AlarmManager.class);SharedPreferences p=prefs(c);
        // Read a complete replacement before touching a previously valid alarm.
        // A transient database failure must not silently erase that alarm.
        List<CourseReminderPlanner.Event> plan = p.getBoolean("enabled", false) && permitted(c)
                ? CourseReminderPlanner.upcoming(courses(c), System.currentTimeMillis(), p.getInt("minutes", 10))
                : Collections.emptyList();
        plan.removeIf(event -> p.getBoolean(deliveredKey(event), false));
        PendingIntent previous = alarm(c, null, PendingIntent.FLAG_NO_CREATE);
        if (previous != null) {
            am.cancel(previous);
            previous.cancel();
        }
        p.edit().putLong("next_at",0).remove("token").remove("planned_at").putBoolean("schedule_error",false).apply();
        if(plan.isEmpty())return;
        CourseReminderPlanner.Event next=plan.get(0);String token=UUID.randomUUID().toString();
        long scheduledAt = Math.max(next.at, System.currentTimeMillis() + 1000);
        // Commit the complete next-alarm identity before allowing delivery.
        if (!p.edit().putString("token", token).putLong("next_at", scheduledAt)
                .putLong("planned_at", next.at).commit()) throw new IllegalStateException("STORAGE_FAILURE");
        PendingIntent operation = alarm(c, token, PendingIntent.FLAG_CANCEL_CURRENT);
        try {
            if (exact(c)) am.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, scheduledAt, operation);
            else am.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, scheduledAt, operation);
        } catch (SecurityException revoked) {
            am.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, scheduledAt, operation);
        }
    }
    static void deliver(Context c,String token)throws Exception{
        SharedPreferences p = prefs(c);
        if (token == null || !token.equals(p.getString("token", ""))) return;
        if (!p.getBoolean("enabled", false) || !permitted(c)) {
            schedule(c);
            return;
        }
        long now = System.currentTimeMillis();
        long at = p.getLong("planned_at", p.getLong("next_at", 0));
        int minutes = p.getInt("minutes", 10);
        // Regenerate from current SQLite rows so edits/deletions cannot send old details.
        List<CourseReminderPlanner.Event> due=CourseReminderPlanner.future(courses(c),at-1,minutes);
        for(CourseReminderPlanner.Event event:due){
            if (event.at != at) break;
            if (now > event.start + DELIVERY_GRACE_MS || now < at - 1000) continue;
            String delivered=deliveredKey(event);if(p.getBoolean(delivered,false))continue;
            String detail=event.course.start+" 上课 · "+(event.course.location.isEmpty()?"教室待确认":event.course.location);
            long remaining=Math.max(0,(event.start-now+59999)/60000);
            PendingIntent open=PendingIntent.getActivity(c,(int)(event.course.id%Integer.MAX_VALUE),new Intent(c,MainActivity.class).setAction(OPEN).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP|Intent.FLAG_ACTIVITY_CLEAR_TOP),PendingIntent.FLAG_UPDATE_CURRENT|PendingIntent.FLAG_IMMUTABLE);
            String timing = now > event.start ? "课程已开始" : remaining == 0 ? "课程即将开始" : "距离上课还有 " + remaining + " 分钟";
            Notification notification=notification(c).setContentTitle(event.course.name).setContentText(detail).setStyle(new NotificationCompat.BigTextStyle().bigText(timing+"\n"+detail)).setContentIntent(open).setAutoCancel(true).build();
            NotificationManager nm = c.getSystemService(NotificationManager.class);
            nm.notify(event.key,(int)(event.course.id%Integer.MAX_VALUE),notification);p.edit().putBoolean(delivered,true).apply();
        }
        schedule(c);
    }

    private static NotificationCompat.Builder notification(Context c) {
        NotificationManager manager = c.getSystemService(NotificationManager.class);
        manager.createNotificationChannel(new NotificationChannel(CHANNEL, "上课提醒", NotificationManager.IMPORTANCE_HIGH));
        return new NotificationCompat.Builder(c, CHANNEL).setSmallIcon(R.drawable.ic_stat_course)
                .setCategory(NotificationCompat.CATEGORY_REMINDER).setVisibility(NotificationCompat.VISIBILITY_PRIVATE);
    }

    static boolean sendTestNotification(Context c) {
        if (!permitted(c)) return false;
        Notification test = notification(c).setContentTitle("上课提醒测试")
                .setContentText("看到这条通知，说明上课提醒渠道可用。")
                .setTimeoutAfter(60_000L).build();
        c.getSystemService(NotificationManager.class).notify("course-reminder-test", 8741, test);
        return true;
    }
}
