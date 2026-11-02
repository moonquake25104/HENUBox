package com.xique.assistant;

import android.content.*;

public final class CourseReminderReceiver extends BroadcastReceiver {
    @Override public void onReceive(Context context,Intent intent){
        if(intent==null)return;String action=intent.getAction();
        if(!CourseReminders.FIRE.equals(action)&&!Intent.ACTION_BOOT_COMPLETED.equals(action)&&!Intent.ACTION_TIME_CHANGED.equals(action)&&!Intent.ACTION_TIMEZONE_CHANGED.equals(action)&&!Intent.ACTION_MY_PACKAGE_REPLACED.equals(action)&&!"android.app.action.SCHEDULE_EXACT_ALARM_PERMISSION_STATE_CHANGED".equals(action))return;
        PendingResult pending=goAsync();Context app=context.getApplicationContext();
        CourseReminders.IO.execute(()->{try{if(CourseReminders.FIRE.equals(action))CourseReminders.deliver(app,intent.getStringExtra("token"));else CourseReminders.schedule(app);}catch(Exception ignored){CourseReminders.prefs(app).edit().putBoolean("schedule_error",true).apply();}finally{pending.finish();}});
    }
}
