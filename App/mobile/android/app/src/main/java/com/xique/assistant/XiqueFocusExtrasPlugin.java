package com.xique.assistant;

import android.app.Activity;
import android.content.*;
import android.provider.Settings;
import androidx.activity.result.ActivityResult;
import com.getcapacitor.*;
import com.getcapacitor.annotation.*;
import com.xique.assistant.focus.*;
import java.util.concurrent.*;

@CapacitorPlugin(name="XiqueFocusExtras")
public final class XiqueFocusExtrasPlugin extends Plugin {
    private final ExecutorService io=Executors.newSingleThreadExecutor();
    private boolean trusted(PluginCall call){if(!SessionPolicy.trustedPage(getBridge().getWebView().getUrl())||getBridge().getServerUrl()!=null){call.reject("Untrusted page","UNTRUSTED_ORIGIN");return false;}return true;}
    private interface Work{void run()throws Exception;}
    private void execute(PluginCall call,Work work){io.execute(()->{try{work.run();JSObject result=new JSObject(FocusExtras.state(getContext()).toString());getActivity().runOnUiThread(()->call.resolve(result));}catch(Exception e){String m=e.getMessage();call.reject("Focus settings failed",e instanceof IllegalArgumentException&&m!=null&&m.matches("[A-Z_]+")?m:"MEDIA_UNAVAILABLE");}});}
    @PluginMethod public void getState(PluginCall call){getActivity().runOnUiThread(()->{if(trusted(call))execute(call,()->{BridgeArgs.exact(call);FocusEngine e=FocusEngine.get(getContext());FocusDnd.sync(getContext(),e.isForegroundAttached()?e.getState().optJSONObject("timer"):null);});});}
    @PluginMethod public void saveAudio(PluginCall call){getActivity().runOnUiThread(()->{if(trusted(call))execute(call,()->{JSObject d=BridgeArgs.exact(call,"sound","volume");long volume=BridgeArgs.integer(d,"volume");if(volume<0||volume>100)throw new IllegalArgumentException("INVALID_ARGUMENT");FocusExtras.saveAudio(getContext(),BridgeArgs.text(d,"sound",16),(int)volume);getActivity().runOnUiThread(()->{try{FocusExtras.sync(getContext());}catch(Exception ignored){}});});});}
    @PluginMethod public void importAudio(PluginCall call){getActivity().runOnUiThread(()->{if(!trusted(call))return;try{BridgeArgs.exact(call);startActivityForResult(call,new Intent(Intent.ACTION_OPEN_DOCUMENT).setType("audio/*").addCategory(Intent.CATEGORY_OPENABLE),"audioResult");}catch(Exception e){call.reject("File chooser unavailable","MEDIA_UNAVAILABLE");}});}
    @ActivityCallback private void audioResult(PluginCall call,ActivityResult result){if(call==null)return;getActivity().runOnUiThread(()->{if(!trusted(call))return;if(result.getResultCode()!=Activity.RESULT_OK||result.getData()==null||result.getData().getData()==null){call.reject("Cancelled","CANCELLED");return;}android.net.Uri uri=result.getData().getData();execute(call,()->{FocusExtras.importAudio(getContext(),uri);getActivity().runOnUiThread(()->{try{FocusExtras.sync(getContext());}catch(Exception ignored){}});});});}
    @PluginMethod public void dndChoice(PluginCall call){getActivity().runOnUiThread(()->{if(!trusted(call))return;try{JSObject d=BridgeArgs.exact(call,"choice");String choice=BridgeArgs.text(d,"choice",16);if(!choice.equals("enabled")&&!choice.equals("declined"))throw new IllegalArgumentException("INVALID_ARGUMENT");FocusExtras.prefs(getContext()).edit().putString("dnd_choice",choice).putBoolean("dnd_new_consent",choice.equals("enabled")).commit();if(choice.equals("enabled")&&!FocusDnd.access(getContext()))getActivity().startActivity(new Intent(Settings.ACTION_NOTIFICATION_POLICY_ACCESS_SETTINGS));execute(call,()->{FocusExtras.sync(getContext());});}catch(Exception e){call.reject("DND settings unavailable",e instanceof IllegalArgumentException?"INVALID_ARGUMENT":"DND_UNAVAILABLE");}});}
    @Override protected void handleOnDestroy(){io.shutdown();super.handleOnDestroy();}
}
