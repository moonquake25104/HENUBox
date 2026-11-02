package com.xique.assistant.focus;

import android.app.*;
import android.content.*;
import android.content.pm.ServiceInfo;
import android.os.*;
import androidx.core.app.NotificationCompat;
import com.xique.assistant.MainActivity;
import com.xique.assistant.R;
import org.json.JSONObject;

/** User-started local timer. No network, push service or school credentials. */
public final class FocusService extends Service {
    private static final int ONGOING=7310,FINISHED=7311;
    private final Handler handler=new Handler(Looper.getMainLooper());
    private PowerManager.WakeLock wake;
    private String rendered="";
    private FocusEngine engine;
    private FocusAudio audio;
    private final Runnable ticker=new Runnable() { public void run(){ update();if(engine!=null)handler.postDelayed(this,1000); } };
    @Override public IBinder onBind(Intent intent){return null;}
    @Override public void onCreate(){super.onCreate();engine=FocusEngine.get(this);audio=new FocusAudio(this);}
    public static void sync(Context context,JSONObject state) {
        if(state.optJSONObject("timer")==null)context.stopService(new Intent(context,FocusService.class));
        else context.startForegroundService(new Intent(context,FocusService.class));
    }
    @Override public int onStartCommand(Intent intent,int flags,int id) {
        try {
            if(intent!=null&&intent.hasExtra("control"))engine.notificationAction(intent.getStringExtra("control"),intent.getStringExtra("session"));
            JSONObject state=engine.getState();
            if(state.optJSONObject("timer")!=null)getSystemService(NotificationManager.class).cancel(FINISHED);
            Notification notification=notification(state);
            if(Build.VERSION.SDK_INT>=34)startForeground(ONGOING,notification,ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE|("off".equals(FocusExtras.prefs(this).getString("sound","off"))?0:ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PLAYBACK));else startForeground(ONGOING,notification);
            engine.setForegroundAttached(true);handler.removeCallbacks(ticker);update();
            if(engine!=null)handler.postDelayed(ticker,1000);
        }catch(Exception error){stopSelf();}
        return START_NOT_STICKY;
    }
    private PendingIntent open() {
        return PendingIntent.getActivity(this,7310,new Intent(this,MainActivity.class).setAction("com.xique.assistant.OPEN_FOCUS").addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP|Intent.FLAG_ACTIVITY_CLEAR_TOP),PendingIntent.FLAG_UPDATE_CURRENT|PendingIntent.FLAG_IMMUTABLE);
    }
    private PendingIntent control(String action,String session) {
        Intent intent=new Intent(this,FocusService.class).setAction("com.xique.assistant.focus."+action+"."+session).putExtra("control",action).putExtra("session",session);
        return PendingIntent.getForegroundService(this,0,intent,PendingIntent.FLAG_UPDATE_CURRENT|PendingIntent.FLAG_IMMUTABLE);
    }
    private Notification notification(JSONObject state)throws Exception {
        NotificationManager nm=getSystemService(NotificationManager.class);
        NotificationChannel channel=new NotificationChannel("focus-active","专注计时",NotificationManager.IMPORTANCE_LOW);channel.setSound(null,null);nm.createNotificationChannel(channel);
        JSONObject t=state.optJSONObject("timer");
        NotificationCompat.Builder b=new NotificationCompat.Builder(this,"focus-active").setSmallIcon(R.drawable.ic_stat_focus).setContentIntent(open()).setOngoing(true).setOnlyAlertOnce(true).setSilent(true).setCategory(NotificationCompat.CATEGORY_PROGRESS).setVisibility(NotificationCompat.VISIBILITY_PRIVATE);
        if(t==null)return b.setContentTitle("专注已结束").build();
        String mode=t.getString("mode"),status=t.getString("status"),label=t.getString("label");boolean running="RUNNING".equals(status);
        String title="focus".equals(mode)?"专注中":"countup".equals(mode)?"自由专注":"休息中";
        if(!running)title="INTERRUPTED".equals(status)?"专注已中断":"计时已暂停";
        b.setContentTitle(title+(label.isEmpty()?"":" · "+label));
        long ms="countup".equals(mode)?t.getLong("elapsed_ms"):t.getLong("remaining_ms");
        b.setContentText(String.format(java.util.Locale.ROOT,"%02d:%02d · 点击返回HENUBox",ms/60000,ms/1000%60));
        if(running)b.setWhen(System.currentTimeMillis()+("countup".equals(mode)?-ms:ms)).setUsesChronometer(true).setChronometerCountDown(!"countup".equals(mode));
        b.addAction(0,running?"暂停":"继续",control(running?"pause":"resume",t.getString("id"))).addAction(0,"结束",control("finish",t.getString("id")));
        return b.build();
    }
    private void alert(JSONObject item,JSONObject prefs) {
        try {
            boolean sound=prefs.optBoolean("sound",true),vibrate=prefs.optBoolean("vibrate",true);
            String id="focus-done-"+(sound?"s":"q")+(vibrate?"v":"n");NotificationManager nm=getSystemService(NotificationManager.class);
            NotificationChannel channel=new NotificationChannel(id,"专注结束"+(sound?" · 声音":"")+(vibrate?" · 振动":""),NotificationManager.IMPORTANCE_HIGH);
            if(!sound)channel.setSound(null,null);channel.enableVibration(vibrate);nm.createNotificationChannel(channel);
            String mode=item.optString("mode");String title="focus".equals(mode)?"一个番茄完成了":"countup".equals(mode)?"自由专注已达 24 小时":"休息结束了";
            nm.notify(FINISHED,new NotificationCompat.Builder(this,id).setSmallIcon(R.drawable.ic_stat_focus).setContentTitle(title).setContentText("打开HENUBox，安排下一段时间。").setContentIntent(open()).setAutoCancel(true).setVisibility(NotificationCompat.VISIBILITY_PRIVATE).build());
        }catch(SecurityException denied){/* Timer and records work without notification permission. */}
    }
    private void update() {
        try {
            JSONObject s=engine.tick(),done=engine.takeAlert();
            JSONObject t=s.optJSONObject("timer");
            FocusDnd.sync(this,t);audio.sync(t);
            if(done!=null)alert(done,s.getJSONObject("settings"));
            if(t==null){release();stopForeground(STOP_FOREGROUND_REMOVE);stopSelf();handler.removeCallbacks(ticker);return;}
            boolean running="RUNNING".equals(t.getString("status"));
            String key=t.getString("id")+t.getString("status");
            if(!key.equals(rendered))release();
            if(running&&(wake==null||!wake.isHeld())) {
                wake=getSystemService(PowerManager.class).newWakeLock(PowerManager.PARTIAL_WAKE_LOCK,"Xique:FocusTimer");wake.setReferenceCounted(false);wake.acquire(Math.min(86500000L,t.getLong("remaining_ms")+60000));
            }else if(!running)release();
            if(!key.equals(rendered)){getSystemService(NotificationManager.class).notify(ONGOING,notification(s));rendered=key;}
        }catch(Exception error){release();stopForeground(STOP_FOREGROUND_REMOVE);stopSelf();}
    }
    private void release(){if(wake!=null&&wake.isHeld())wake.release();wake=null;}
    @Override public void onDestroy(){handler.removeCallbacks(ticker);release();if(audio!=null)audio.close();FocusDnd.sync(this,null);if(engine!=null)engine.setForegroundAttached(false);engine=null;super.onDestroy();}
}
