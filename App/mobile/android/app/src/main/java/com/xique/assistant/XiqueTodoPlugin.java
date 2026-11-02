package com.xique.assistant;

import com.getcapacitor.*;
import com.getcapacitor.annotation.*;
import com.xique.assistant.todo.*;
import org.json.JSONObject;
import java.util.concurrent.*;

@CapacitorPlugin(name="XiqueTodo")
public final class XiqueTodoPlugin extends Plugin {
    private final ExecutorService io=Executors.newSingleThreadExecutor();
    private boolean trusted(PluginCall c){if(!SessionPolicy.trustedPage(getBridge().getWebView().getUrl())||getBridge().getServerUrl()!=null){c.reject("Untrusted page","UNTRUSTED_ORIGIN");return false;}return true;}
    private String text(JSObject d,String key)throws Exception{Object v=d.get(key);if(!(v instanceof String))throw new IllegalArgumentException("INVALID_ARGUMENT");return (String)v;}
    private boolean bool(JSObject d,String key)throws Exception{Object v=d.get(key);if(!(v instanceof Boolean))throw new IllegalArgumentException("INVALID_ARGUMENT");return (Boolean)v;}
    @PluginMethod public void getState(PluginCall c){run(c,"state");}
    @PluginMethod public void create(PluginCall c){run(c,"create");}
    @PluginMethod public void update(PluginCall c){run(c,"update");}
    @PluginMethod public void setCompleted(PluginCall c){run(c,"complete");}
    @PluginMethod public void remove(PluginCall c){run(c,"remove");}
    private void run(PluginCall c,String method){getActivity().runOnUiThread(()->{
        if(!trusted(c))return;
        io.execute(()->{try{
            TodoStore store=TodoStore.get(getContext());JSONObject state;JSObject d;
            switch(method){
                case "state":BridgeArgs.exact(c);state=store.state();break;
                case "create":d=BridgeArgs.exact(c,"id","title","note","due_date","important");state=store.create(BridgeArgs.uuid(d,"id"),text(d,"title"),text(d,"note"),text(d,"due_date"),bool(d,"important"));break;
                case "update":d=BridgeArgs.exact(c,"id","expected_revision","title","note","due_date","important");state=store.update(BridgeArgs.uuid(d,"id"),BridgeArgs.integer(d,"expected_revision"),text(d,"title"),text(d,"note"),text(d,"due_date"),bool(d,"important"));break;
                case "complete":d=BridgeArgs.exact(c,"id","expected_revision","completed");state=store.complete(BridgeArgs.uuid(d,"id"),BridgeArgs.integer(d,"expected_revision"),bool(d,"completed"));break;
                case "remove":d=BridgeArgs.exact(c,"id","expected_revision");state=store.remove(BridgeArgs.uuid(d,"id"),BridgeArgs.integer(d,"expected_revision"));break;
                default:throw new IllegalArgumentException("INVALID_ARGUMENT");
            }
            JSObject value=new JSObject(state.toString());getActivity().runOnUiThread(()->c.resolve(value));
        }catch(Exception e){String message=e.getMessage();if(e instanceof IllegalArgumentException&&message!=null&&message.matches("[A-Z_]+"))c.reject("Todo operation rejected",message);else BridgeArgs.reject(c,e);}});
    });}
    @Override protected void handleOnDestroy(){io.shutdown();super.handleOnDestroy();}
}
