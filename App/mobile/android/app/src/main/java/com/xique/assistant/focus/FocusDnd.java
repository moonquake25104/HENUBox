package com.xique.assistant.focus;

import android.app.*;
import android.content.*;
import android.net.Uri;
import android.os.Build;
import android.service.notification.*;
import org.json.JSONObject;

/** Own rule only: never changes the user's global filter or somebody else's rule. */
public final class FocusDnd extends ConditionProviderService {
    private static final Uri CONDITION=Uri.parse("condition://com.xique.assistant/focus");
    private static final String RULE_NAME="HENUBox · 专注勿扰";
    private static FocusDnd provider;private static String last="";
    public static boolean access(Context c){return c.getSystemService(NotificationManager.class).isNotificationPolicyAccessGranted();}
    public static boolean active(Context c){try{String id=FocusExtras.prefs(c).getString("dnd_rule",null);NotificationManager nm=c.getSystemService(NotificationManager.class);AutomaticZenRule r=id==null?null:nm.getAutomaticZenRule(id);return access(c)&&r!=null&&r.isEnabled()&&(Build.VERSION.SDK_INT>=35?nm.getAutomaticZenRuleState(id)==Condition.STATE_TRUE:"true".equals(last));}catch(Exception e){return false;}}
    public static synchronized void sync(Context c,JSONObject timer){
        boolean enabled="enabled".equals(FocusExtras.prefs(c).getString("dnd_choice","undecided"));
        boolean run=enabled&&timer!=null&&"RUNNING".equals(timer.optString("status"))&&("focus".equals(timer.optString("mode"))||"countup".equals(timer.optString("mode")));
        if(!access(c)){last="";return;}
        try {
            NotificationManager nm=c.getSystemService(NotificationManager.class);String id=FocusExtras.prefs(c).getString("dnd_rule",null);
            if(enabled&&FocusExtras.prefs(c).getBoolean("dnd_new_consent",false)){
                AutomaticZenRule previous=id==null?null:nm.getAutomaticZenRule(id);
                if(id!=null&&previous==null){FocusExtras.prefs(c).edit().remove("dnd_rule").commit();id=null;}
                else if(previous!=null&&!previous.isEnabled()){previous.setEnabled(true);nm.updateAutomaticZenRule(id,previous);}
                FocusExtras.prefs(c).edit().putBoolean("dnd_new_consent",false).commit();last="";
            }
            // A user may remove this rule in system settings. Do not recreate it every tick.
            if(id!=null){
                AutomaticZenRule own=nm.getAutomaticZenRule(id);
                if(own==null){last="";return;}
                // Rename our existing rule after an app update; keep its settings.
                if(!RULE_NAME.equals(own.getName())){own.setName(RULE_NAME);nm.updateAutomaticZenRule(id,own);}
            }
            if(id==null&&run){
                AutomaticZenRule rule=new AutomaticZenRule(RULE_NAME,new ComponentName(c,FocusDnd.class),CONDITION,NotificationManager.INTERRUPTION_FILTER_PRIORITY,true);
                if(Build.VERSION.SDK_INT>=29)rule.setZenPolicy(new ZenPolicy.Builder().allowAlarms(true).allowMedia(true).allowCalls(ZenPolicy.PEOPLE_TYPE_STARRED).allowMessages(ZenPolicy.PEOPLE_TYPE_NONE).allowRepeatCallers(true).build());
                id=nm.addAutomaticZenRule(rule);if(id==null)return;FocusExtras.prefs(c).edit().putString("dnd_rule",id).commit();last="";
            }
            if(id==null)return;String key=Boolean.toString(run);if(last.equals(key))return;
            Condition condition=new Condition(CONDITION,"HENUBox 专注",run?Condition.STATE_TRUE:Condition.STATE_FALSE);
            if(Build.VERSION.SDK_INT>=29)nm.setAutomaticZenRuleState(id,condition);else if(provider!=null)provider.notifyCondition(condition);else {requestRebind(new ComponentName(c,FocusDnd.class));return;}
            last=key;
        }catch(Exception ignored){last="";}
    }
    @Override public void onConnected(){provider=this;last="";restore();}
    @Override public void onSubscribe(Uri id){if(CONDITION.equals(id)){last="";restore();}}
    private void restore(){try{FocusEngine engine=FocusEngine.get(this);sync(this,engine.isForegroundAttached()?engine.getState().optJSONObject("timer"):null);}catch(Exception ignored){sync(this,null);}}
    @Override public void onUnsubscribe(Uri id){}
    @Override public void onDestroy(){provider=null;last="";super.onDestroy();}
}
