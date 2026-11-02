package com.xique.assistant;
import android.content.Context;
import androidx.test.platform.app.InstrumentationRegistry;
import org.json.*;
import org.junit.Test;
import java.io.File;
import java.nio.file.Files;
import java.security.MessageDigest;
import java.util.*;
import static org.junit.Assert.*;

/** Opt-in, actual main-app legacy migration. Never connects to the school. */
public class LocalM2DeviceTest {
    @Test public void realLegacySnapshotIsPreservedByLocalBootstrap() throws Exception {
        int expected=Integer.parseInt(InstrumentationRegistry.getArguments().getString("expected_legacy_count","-1"));
        assertTrue("Explicit legacy count required",expected>=0);
        Context context=InstrumentationRegistry.getInstrumentation().getTargetContext();
        File original=new File(context.getNoBackupFilesDir(),"timetable-v1.sqlite");
        assertTrue(original.exists());byte[] before=MessageDigest.getInstance("SHA-256").digest(Files.readAllBytes(original.toPath()));
        JSONObject owner=new SessionVault(context).read();
        JSONObject snapshot=new TimetableStore(context).read(owner.getLong("generation"));assertNotNull(snapshot);
        JSONArray legacy=snapshot.getJSONArray("courses");assertEquals(expected,legacy.length());
        com.xique.assistant.henu.ops.HenuService service=LocalCore.service(context);
        LocalCore.migrateLegacy(context,service);JSONObject boot=service.bootstrap();JSONArray courses=boot.getJSONArray("courses");
        assertEquals(expected,courses.length());
        Map<Long,String> canonical=new HashMap<>();for(int i=0;i<legacy.length();i++)canonical.put(legacy.getJSONObject(i).getLong("id"),canonical(legacy.getJSONObject(i)));
        for(int i=0;i<courses.length();i++)assertEquals("Legacy course changed",canonical.get(courses.getJSONObject(i).getLong("id")),canonical(courses.getJSONObject(i)));
        UUID.fromString(boot.getString("workspace_id"));assertEquals("LOCAL",boot.getString("mode"));
        assertTrue(boot.getJSONObject("capabilities").getBoolean("can_commit"));
        LocalCore.migrateLegacy(context,service);assertEquals(expected,service.bootstrap().getJSONArray("courses").length());
        assertArrayEquals(before,MessageDigest.getInstance("SHA-256").digest(Files.readAllBytes(original.toPath())));
        System.out.println("LOCAL_M2_LEGACY_PRESERVED count="+expected+" original_file_unchanged=true");
    }
    private static String canonical(JSONObject object) throws Exception {
        List<String> keys=new ArrayList<>();object.keys().forEachRemaining(keys::add);Collections.sort(keys);
        JSONArray out=new JSONArray();for(String key:keys)out.put(new JSONArray().put(key).put(object.get(key)));return out.toString();
    }
}
