package com.xique.assistant;

import android.Manifest;
import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.content.pm.PackageManager;
import android.os.Build;
import android.provider.Settings;
import com.getcapacitor.*;
import com.getcapacitor.annotation.*;

@CapacitorPlugin(name="XiqueCourseReminders",permissions={@Permission(alias="notifications",strings={Manifest.permission.POST_NOTIFICATIONS})})
public final class XiqueCourseRemindersPlugin extends Plugin {
    private boolean trusted(PluginCall call){if(!SessionPolicy.trustedPage(getBridge().getWebView().getUrl())||getBridge().getServerUrl()!=null){call.reject("Untrusted page","UNTRUSTED_ORIGIN");return false;}return true;}
    @PluginMethod public void getState(PluginCall call){getActivity().runOnUiThread(()->readState(call));}
    private void readState(PluginCall call){if(!trusted(call))return;try{BridgeArgs.exact(call);Activity a=getActivity();boolean open=CourseReminders.OPEN.equals(a.getIntent().getAction());if(open)a.getIntent().setAction(Intent.ACTION_MAIN);CourseReminders.IO.execute(()->{try{JSObject value=new JSObject(CourseReminders.state(getContext()).toString());value.put("open_timetable",open);call.resolve(value);}catch(Exception e){BridgeArgs.reject(call,e);}});}catch(Exception e){BridgeArgs.reject(call,e);}}
    @PluginMethod public void configure(PluginCall call){getActivity().runOnUiThread(()->configureOnUi(call));}
    private void configureOnUi(PluginCall call){if(!trusted(call))return;try{JSObject data=BridgeArgs.exact(call,"enabled","minutes");if(!(data.get("enabled") instanceof Boolean))throw new IllegalArgumentException("INVALID_ARGUMENT");long minutes=BridgeArgs.integer(data,"minutes");if(minutes<0||minutes>120)throw new IllegalArgumentException("INVALID_ARGUMENT");
        if (data.getBoolean("enabled") && Build.VERSION.SDK_INT >= 33
                && getContext().checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            try {
                PermissionState permission = getPermissionState("notifications");
                if (permission == PermissionState.PROMPT || permission == PermissionState.PROMPT_WITH_RATIONALE) {
                    requestPermissionForAlias("notifications", call, "afterPermission");
                    return;
                }
            } catch (RuntimeException failed) {
                android.util.Log.w("HENUBoxReminders", "Unable to request notification permission", failed);
            }
        }
        save(call);
    }catch(Exception e){BridgeArgs.reject(call,e);}}
    @PermissionCallback private void afterPermission(PluginCall call){if(trusted(call))save(call);}
    private void save(PluginCall call){CourseReminders.IO.execute(()->{try{boolean enabled=call.getBoolean("enabled",false);int minutes=call.getInt("minutes",10);if(!CourseReminders.prefs(getContext()).edit().putBoolean("enabled",enabled).putInt("minutes",minutes).commit())throw new IllegalStateException("STORAGE_FAILURE");try{CourseReminders.schedule(getContext());}catch(Exception failed){CourseReminders.prefs(getContext()).edit().putBoolean("schedule_error",true).apply();}call.resolve(new JSObject(CourseReminders.state(getContext()).toString()));}catch(Exception e){BridgeArgs.reject(call,e);}});}
    @PluginMethod public void openExactSettings(PluginCall call){getActivity().runOnUiThread(()->openSettings(call,true));}
    @PluginMethod public void openNotificationSettings(PluginCall call){getActivity().runOnUiThread(()->openSettings(call,false));}
    @PluginMethod public void sendTestNotification(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!trusted(call)) return;
            try {
                BridgeArgs.exact(call);
                if (CourseReminders.sendTestNotification(getContext())) call.resolve();
                else call.reject("Course notifications are disabled", "NOTIFICATIONS_DISABLED");
            } catch (Exception e) {
                BridgeArgs.reject(call, e);
            }
        });
    }
    private void openSettings(PluginCall call,boolean exact){if(!trusted(call))return;try{BridgeArgs.exact(call);getActivity().runOnUiThread(()->{try{Intent i=exact&&Build.VERSION.SDK_INT>=31?new Intent(Settings.ACTION_REQUEST_SCHEDULE_EXACT_ALARM,Uri.parse("package:"+getContext().getPackageName())):new Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE,getContext().getPackageName());getActivity().startActivity(i);call.resolve();}catch(Exception e){call.reject("Unable to open settings","SETTINGS_UNAVAILABLE");}});}catch(Exception e){BridgeArgs.reject(call,e);}}
}
