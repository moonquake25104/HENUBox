package com.xique.assistant.focus;

import android.content.*;
import android.media.MediaMetadataRetriever;
import android.net.Uri;
import android.provider.OpenableColumns;
import org.json.JSONObject;
import java.io.*;

/** Private, bounded audio copy. No shared-library permission or network audio. */
public final class FocusExtras {
    public static final long MAX_AUDIO=20L*1024*1024;
    public static android.content.SharedPreferences prefs(Context c){return c.getSharedPreferences("focus-extras",Context.MODE_PRIVATE);}
    public static boolean validSound(String s){return s!=null&&java.util.Arrays.asList("off","rain","waves","pink","brown","custom").contains(s);}
    public static synchronized JSONObject state(Context c)throws Exception {
        android.content.SharedPreferences p=prefs(c);
        return new JSONObject().put("sound",p.getString("sound","off")).put("volume",p.getInt("volume",35))
            .put("custom_name",p.getString("custom_name","")).put("custom_available",new File(c.getFilesDir(),"focus-audio/custom.audio").isFile())
            .put("audio_status",FocusAudio.status()).put("dnd_choice",p.getString("dnd_choice","undecided"))
            .put("dnd_access",FocusDnd.access(c)).put("dnd_active",FocusDnd.active(c));
    }
    public static synchronized void saveAudio(Context c,String sound,int volume){
        if(!validSound(sound)||volume<0||volume>100||("custom".equals(sound)&&!new File(c.getFilesDir(),"focus-audio/custom.audio").isFile()))throw new IllegalArgumentException("INVALID_ARGUMENT");
        prefs(c).edit().putString("sound",sound).putInt("volume",volume).commit();
    }
    public static synchronized void importAudio(Context c,Uri uri)throws Exception {
        String mime=c.getContentResolver().getType(uri);
        if(mime==null||!mime.startsWith("audio/"))throw new IllegalArgumentException("AUDIO_FORMAT");
        String name="我的音频";
        try(android.database.Cursor cursor=c.getContentResolver().query(uri,new String[]{OpenableColumns.DISPLAY_NAME,OpenableColumns.SIZE},null,null,null)){
            if(cursor!=null&&cursor.moveToFirst()){
                if(!cursor.isNull(1)&&cursor.getLong(1)>MAX_AUDIO)throw new IllegalArgumentException("AUDIO_LIMIT");
                String n=cursor.getString(0);if(n!=null)name=n.replaceAll("[\\p{Cntrl}]","");
            }
        }
        if(name.codePointCount(0,name.length())>80)name=name.substring(0,name.offsetByCodePoints(0,80));
        File dir=new File(c.getFilesDir(),"focus-audio");if(!dir.isDirectory()&&!dir.mkdirs())throw new IOException("storage");
        File temp=new File(dir,"import.tmp"),target=new File(dir,"custom.audio");
        try {
            try(InputStream in=c.getContentResolver().openInputStream(uri);FileOutputStream out=new FileOutputStream(temp)){
                if(in==null)throw new IOException("input");byte[] bytes=new byte[32768];int n;long size=0;
                while((n=in.read(bytes))!=-1){size+=n;if(size>MAX_AUDIO)throw new IllegalArgumentException("AUDIO_LIMIT");out.write(bytes,0,n);}out.getFD().sync();
                if(size==0)throw new IllegalArgumentException("AUDIO_FORMAT");
            }
            MediaMetadataRetriever meta=new MediaMetadataRetriever();
            try{
                meta.setDataSource(temp.getAbsolutePath());String d=meta.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION);
                long duration=d==null?0:Long.parseLong(d);if(duration<1000||duration>30*60*1000L)throw new IllegalArgumentException("AUDIO_LIMIT");
            }catch(IllegalArgumentException e){throw e;}catch(Exception e){throw new IllegalArgumentException("AUDIO_FORMAT");}
            finally{meta.release();} // close()/AutoCloseable only exists from API29; minSdk is 28.
            java.nio.file.Files.move(temp.toPath(),target.toPath(),java.nio.file.StandardCopyOption.REPLACE_EXISTING,java.nio.file.StandardCopyOption.ATOMIC_MOVE);
            prefs(c).edit().putString("custom_name",name).putString("sound","custom").putLong("audio_version",System.currentTimeMillis()).commit();
        }finally{temp.delete();}
    }
    public static void sync(Context c)throws Exception {JSONObject s=FocusEngine.get(c).getState();FocusService.sync(c,s);}
}
