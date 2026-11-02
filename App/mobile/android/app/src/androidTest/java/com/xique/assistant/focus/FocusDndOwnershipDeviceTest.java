package com.xique.assistant.focus;

import android.app.*;
import android.content.*;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.json.JSONObject;
import org.junit.*;
import org.junit.runner.RunWith;
import java.util.*;
import static org.junit.Assert.*;

/** Explicit emulator-only integration tests, never requests policy access itself. */
@RunWith(AndroidJUnit4.class)
public class FocusDndOwnershipDeviceTest {
    Context base,isolated;NotificationManager nm;String pref;Set<String> created=new HashSet<>();
    @Before public void setup(){Assume.assumeTrue(android.os.Build.HARDWARE.contains("ranchu")||android.os.Build.HARDWARE.contains("goldfish"));base=InstrumentationRegistry.getInstrumentation().getTargetContext();Assume.assumeTrue(FocusDnd.access(base));nm=base.getSystemService(NotificationManager.class);pref="dnd-test-"+UUID.randomUUID();isolated=new ContextWrapper(base){public SharedPreferences getSharedPreferences(String name,int mode){return base.getSharedPreferences(pref,mode);}};FocusExtras.prefs(isolated).edit().putString("dnd_choice","enabled").putBoolean("dnd_new_consent",true).commit();}
    @After public void cleanup(){if(nm!=null)for(String id:created)nm.removeAutomaticZenRule(id);if(base!=null)base.deleteSharedPreferences(pref);}
    private JSONObject timer()throws Exception{return new JSONObject().put("id",UUID.randomUUID().toString()).put("mode","focus").put("status","RUNNING");}
    private String id(){String id=FocusExtras.prefs(isolated).getString("dnd_rule",null);assertNotNull(id);created.add(id);return id;}
    @Test public void removedRuleRequiresFreshConsent()throws Exception{
        JSONObject timer=timer();FocusDnd.sync(isolated,timer);String old=id();assertNotNull(nm.getAutomaticZenRule(old));nm.removeAutomaticZenRule(old);
        FocusDnd.sync(isolated,timer);assertNull(nm.getAutomaticZenRule(old));assertEquals(old,FocusExtras.prefs(isolated).getString("dnd_rule",null));
        FocusExtras.prefs(isolated).edit().putBoolean("dnd_new_consent",true).commit();FocusDnd.sync(isolated,timer);String fresh=id();assertNotEquals(old,fresh);assertNotNull(nm.getAutomaticZenRule(fresh));FocusDnd.sync(isolated,null);
    }
    @Test public void disabledRuleIsNotForceEnabled()throws Exception{
        JSONObject timer=timer();FocusDnd.sync(isolated,timer);String id=id();AutomaticZenRule rule=nm.getAutomaticZenRule(id);rule.setEnabled(false);nm.updateAutomaticZenRule(id,rule);
        FocusDnd.sync(isolated,timer);assertFalse(nm.getAutomaticZenRule(id).isEnabled());FocusDnd.sync(isolated,null);
    }
}
