package com.xique.assistant.focus;

import android.content.*;
import android.media.MediaPlayer;
import androidx.core.content.FileProvider;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.json.JSONObject;
import org.junit.*;
import org.junit.runner.RunWith;
import java.io.*;
import java.util.*;
import java.util.concurrent.*;
import static org.junit.Assert.*;

@RunWith(AndroidJUnit4.class)
public class FocusExtrasDeviceTest {
    Context base,isolated;File dir;String pref;
    @Before public void setup(){base=InstrumentationRegistry.getInstrumentation().getTargetContext();dir=new File(base.getCacheDir(),"focus-extras-"+UUID.randomUUID());dir.mkdirs();pref="test-"+dir.getName();isolated=new ContextWrapper(base){public File getFilesDir(){return dir;}public android.content.SharedPreferences getSharedPreferences(String name,int mode){return base.getSharedPreferences(pref,mode);}};}
    @After public void clean(){FocusExtras.prefs(isolated).edit().clear().commit();delete(dir);base.deleteSharedPreferences(pref);}
    private void delete(File f){if(f.isDirectory())for(File c:Objects.requireNonNull(f.listFiles()))delete(c);f.delete();}
    @Test public void defaultIsSilentAndConsentRequired()throws Exception {JSONObject s=FocusExtras.state(isolated);assertEquals("off",s.getString("sound"));assertEquals("undecided",s.getString("dnd_choice"));FocusDnd.sync(isolated,null);assertNull(FocusExtras.prefs(isolated).getString("dnd_rule",null));}
    @Test public void audioSettingsPersistAndRejectMissingCustom()throws Exception {FocusExtras.saveAudio(isolated,"rain",35);assertEquals("rain",FocusExtras.state(isolated).getString("sound"));for(String value:new String[]{"../bad","custom","https://example.org/audio"})try{FocusExtras.saveAudio(isolated,value,30);fail();}catch(IllegalArgumentException expected){}try{FocusExtras.saveAudio(isolated,"off",101);fail();}catch(IllegalArgumentException expected){}assertEquals("rain",FocusExtras.state(isolated).getString("sound"));}
    @Test public void allSmallAssetsDecodeAndLoop()throws Exception {long bytes=0;for(String name:new String[]{"rain","waves","pink","brown"}){try(android.content.res.AssetFileDescriptor fd=base.getAssets().openFd("focus-audio/"+name+".ogg")){bytes+=fd.getLength();MediaPlayer player=new MediaPlayer();try{player.setDataSource(fd.getFileDescriptor(),fd.getStartOffset(),fd.getLength());player.prepare();assertTrue(player.getDuration()>15000&&player.getDuration()<31000);player.setLooping(true);assertTrue(player.isLooping());}finally{player.release();}}}assertTrue(bytes<900000);}
    @Test public void importIsPrivateAndBadReplacementPreservesOriginal()throws Exception {
        File audio=new File(dir,"test.ogg");try(InputStream in=base.getAssets().open("focus-audio/pink.ogg");OutputStream out=new FileOutputStream(audio)){byte[] buffer=new byte[8192];int n;while((n=in.read(buffer))!=-1)out.write(buffer,0,n);}
        android.net.Uri uri=FileProvider.getUriForFile(base,base.getPackageName()+".fileprovider",audio);FocusExtras.importAudio(isolated,uri);
        File target=new File(dir,"focus-audio/custom.audio");byte[] original=java.nio.file.Files.readAllBytes(target.toPath());assertTrue(original.length>100);assertEquals("custom",FocusExtras.state(isolated).getString("sound"));
        try(OutputStream out=new FileOutputStream(audio)){out.write(new byte[]{1,2,3});}try{FocusExtras.importAudio(isolated,uri);fail();}catch(IllegalArgumentException expected){}
        assertArrayEquals(original,java.nio.file.Files.readAllBytes(target.toPath()));assertFalse(new File(dir,"focus-audio/import.tmp").exists());
    }
    @Test public void oversizedImportDoesNotReplace()throws Exception {File audio=new File(dir,"large.mp3");try(RandomAccessFile out=new RandomAccessFile(audio,"rw")){out.setLength(FocusExtras.MAX_AUDIO+1);}try{FocusExtras.importAudio(isolated,FileProvider.getUriForFile(base,base.getPackageName()+".fileprovider",audio));fail();}catch(IllegalArgumentException expected){assertEquals("AUDIO_LIMIT",expected.getMessage());}assertFalse(new File(dir,"focus-audio/custom.audio").exists());}
}
