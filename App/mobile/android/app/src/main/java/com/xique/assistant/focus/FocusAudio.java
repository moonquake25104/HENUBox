package com.xique.assistant.focus;

import android.content.*;
import android.media.*;
import android.os.Handler;
import android.os.Looper;
import org.json.JSONObject;
import java.io.File;

/** One streaming decoder, bounded memory; focus loss pauses until the next user resume. */
public final class FocusAudio implements AutoCloseable {
    private final Context context;private final AudioManager manager;private MediaPlayer player;private AudioFocusRequest request;
    private String key="";private boolean blocked;private static volatile String state="off";
    public static String status(){return state;}
    public FocusAudio(Context c){context=c;manager=c.getSystemService(AudioManager.class);c.registerReceiver(noisy,new IntentFilter(AudioManager.ACTION_AUDIO_BECOMING_NOISY));}
    private final BroadcastReceiver noisy=new BroadcastReceiver(){public void onReceive(Context c,Intent i){interrupt();}};
    private void interrupt(){blocked=true;release();state="interrupted";}
    public void sync(JSONObject timer){
        String source=FocusExtras.prefs(context).getString("sound","off");int volume=FocusExtras.prefs(context).getInt("volume",35);
        boolean run=timer!=null&&"RUNNING".equals(timer.optString("status"))&&("focus".equals(timer.optString("mode"))||"countup".equals(timer.optString("mode")));
        String next=run?timer.optString("id")+source+volume+FocusExtras.prefs(context).getLong("audio_version",0):"";
        if(!next.equals(key)){blocked=false;release();key=next;}
        if(!run||source.equals("off")||volume==0){state="off";return;}
        if(player!=null||blocked)return;
        try {
            AudioAttributes attrs=new AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_MEDIA).setContentType(AudioAttributes.CONTENT_TYPE_MUSIC).build();
            request=new AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN).setAudioAttributes(attrs).setOnAudioFocusChangeListener(change->{if(change!=AudioManager.AUDIOFOCUS_GAIN)interrupt();},new Handler(Looper.getMainLooper())).build();
            if(manager.requestAudioFocus(request)!=AudioManager.AUDIOFOCUS_REQUEST_GRANTED){blocked=true;state="interrupted";release();return;}
            player=new MediaPlayer();player.setAudioAttributes(attrs);
            if(source.equals("custom"))player.setDataSource(new File(context.getFilesDir(),"focus-audio/custom.audio").getAbsolutePath());
            else try(android.content.res.AssetFileDescriptor asset=context.getAssets().openFd("focus-audio/"+source+".ogg")){player.setDataSource(asset.getFileDescriptor(),asset.getStartOffset(),asset.getLength());}
            player.setLooping(true);player.setVolume(volume/100f,volume/100f);
            player.setOnErrorListener((p,w,e)->{interrupt();state="error";return true;});
            player.setOnPreparedListener(p->{if(player==p){p.start();state="playing";}});state="loading";player.prepareAsync();
        }catch(Exception e){blocked=true;release();state="error";}
    }
    private void release(){if(player!=null){player.release();player=null;}if(request!=null){manager.abandonAudioFocusRequest(request);request=null;}}
    public void close(){release();state="off";try{context.unregisterReceiver(noisy);}catch(Exception ignored){}}
}
