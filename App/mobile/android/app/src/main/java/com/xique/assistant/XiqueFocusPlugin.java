package com.xique.assistant;

import android.Manifest;
import android.os.Build;
import android.content.pm.PackageManager;
import androidx.core.app.NotificationManagerCompat;
import com.getcapacitor.*;
import com.getcapacitor.annotation.*;
import com.xique.assistant.focus.*;
import org.json.JSONObject;
import java.util.concurrent.Executors;
import java.util.concurrent.ExecutorService;

@CapacitorPlugin(name="XiqueFocus",permissions={@Permission(alias="notifications",strings={Manifest.permission.POST_NOTIFICATIONS})})
public final class XiqueFocusPlugin extends Plugin {
    private final ExecutorService io=Executors.newSingleThreadExecutor();
    private boolean trusted(PluginCall call) {
        if(!SessionPolicy.trustedPage(getBridge().getWebView().getUrl())||getBridge().getServerUrl()!=null){call.reject("Untrusted page","UNTRUSTED_ORIGIN");return false;}return true;
    }
    @PluginMethod public void getState(PluginCall call){getActivity().runOnUiThread(() -> {if(trusted(call))run(call,"state");});}
    @PluginMethod public void saveSettings(PluginCall call){getActivity().runOnUiThread(() -> {if(trusted(call))run(call,"settings");});}
    @PluginMethod public void command(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if(!trusted(call))return;
            String action=call.getString("action","");
            if(Build.VERSION.SDK_INT>=33&&(action.equals("start")||action.equals("resume"))&&getPermissionState("notifications")==PermissionState.PROMPT)requestPermissionForAlias("notifications",call,"notificationResult");
            else run(call,"command");
        });
    }
    @PermissionCallback private void notificationResult(PluginCall call){getActivity().runOnUiThread(() -> {if(trusted(call))run(call,"command");});}
    private void run(PluginCall call,String method) {
        io.execute(() -> {
            try {
                FocusEngine engine=FocusEngine.get(getContext());JSONObject s;
                if(method.equals("state")){BridgeArgs.exact(call);s=engine.getState();}
                else if(method.equals("settings")){
                    JSObject d=BridgeArgs.exact(call,"expected_revision","settings");s=engine.saveSettings(BridgeArgs.integer(d,"expected_revision"),BridgeArgs.object(d,"settings"));
                }else {
                    JSObject d=BridgeArgs.exact(call,"action","expected_revision","request_id","mode","label");
                    Object mode=d.get("mode"),label=d.get("label");if(!(mode instanceof String)||!(label instanceof String))throw new IllegalArgumentException("INVALID_ARGUMENT");
                    s=engine.command(BridgeArgs.text(d,"action",16),BridgeArgs.integer(d,"expected_revision"),BridgeArgs.uuid(d,"request_id"),(String)mode,(String)label);
                }
                s.put("notification_permission",NotificationManagerCompat.from(getContext()).areNotificationsEnabled());
                if("com.xique.assistant.OPEN_FOCUS".equals(getActivity().getIntent().getAction())) {
                    s.put("open_requested",true);getActivity().getIntent().setAction(null);
                }
                JSObject result=new JSObject(s.toString());
                getActivity().runOnUiThread(() -> {
                    try {
                        if(method.equals("command")||s.optJSONObject("timer")!=null&&!engine.isForegroundAttached())FocusService.sync(getContext(),s);
                        call.resolve(result);
                    }catch(Exception denied){
                        // State has already committed. Pause instead of claiming the start never happened.
                        try {JSONObject t=s.optJSONObject("timer");if(t!=null&&"RUNNING".equals(t.optString("status")))engine.notificationAction("pause",t.optString("id"));}catch(Exception ignored){}
                        call.reject("后台计时启动失败，请重新打开后继续","BACKGROUND_UNAVAILABLE");
                    }
                });
            }catch(Exception e){
                if(e instanceof IllegalArgumentException){String code=e.getMessage();call.reject("Focus operation rejected",code!=null&&code.matches("[A-Z_]+")?code:"INVALID_ARGUMENT");}
                else BridgeArgs.reject(call,e);
            }
        });
    }
    @Override protected void handleOnDestroy(){io.shutdown();super.handleOnDestroy();}
}
