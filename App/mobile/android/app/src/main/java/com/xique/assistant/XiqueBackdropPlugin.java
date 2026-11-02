package com.xique.assistant;

import android.graphics.Color;
import com.getcapacitor.*;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.henubox.glass.BackdropNavHost;

/** Presentation only. DOM buttons remain the only navigation input/state owner. */
@CapacitorPlugin(name="XiqueBackdrop")
public final class XiqueBackdropPlugin extends Plugin {
    private boolean trusted(PluginCall call){if(!SessionPolicy.trustedPage(getBridge().getWebView().getUrl())||getBridge().getServerUrl()!=null){call.reject("Untrusted page","UNTRUSTED_ORIGIN");return false;}return true;}
    private MainActivity activity(){return (MainActivity)getActivity();}
    @PluginMethod public void getCapabilities(PluginCall call){getActivity().runOnUiThread(()->{
        if(!trusted(call))return;
        try{BridgeArgs.exact(call);boolean supported=BackdropNavHost.supported(getActivity());if(supported)activity().ensureBackdropHost();call.resolve(new JSObject().put("supported",supported).put("renderer","compose-backdrop").put("upstream","AndroidLiquidGlass/Backdrop-2.0.1"));}
        catch(Exception error){call.resolve(new JSObject().put("supported",false));}
    });}
    @PluginMethod public void configure(PluginCall call){getActivity().runOnUiThread(()->{
        if(!trusted(call))return;
        try{
            JSObject a=BridgeArgs.exact(call,"x","y","width","height","viewport_width","selected","position","pressed","dark","ink","reduced","sequence");
            float x=number(a,"x"),y=number(a,"y"),width=number(a,"width"),height=number(a,"height"),viewport=number(a,"viewport_width"),position=number(a,"position");long selected=BridgeArgs.integer(a,"selected"),sequence=BridgeArgs.integer(a,"sequence");String ink=BridgeArgs.text(a,"ink",7);
            if(viewport<200||viewport>3000||x<0||y<0||width<180||height<40||height>120||x+width>viewport+2||selected<0||selected>3||position<0||position>3||sequence<0||!(a.get("pressed") instanceof Boolean)||!(a.get("dark") instanceof Boolean)||!(a.get("reduced") instanceof Boolean)||!ink.matches("#[0-9a-fA-F]{6}"))throw new IllegalArgumentException("INVALID_ARGUMENT");
            if(!BackdropNavHost.supported(getActivity())){call.resolve(new JSObject().put("active",false));return;}
            float scale=getBridge().getWebView().getWidth()/viewport;if(y*scale+height*scale>getBridge().getWebView().getHeight()+2)throw new IllegalArgumentException("INVALID_ARGUMENT");
            boolean active=activity().ensureBackdropHost().configure(x,y,width,height,viewport,(int)selected,position,a.getBoolean("pressed"),a.getBoolean("dark"),Color.parseColor(ink),a.getBoolean("reduced"),sequence);
            call.resolve(new JSObject().put("active",active));
        }catch(Exception e){BridgeArgs.reject(call,e);}
    });}
    @PluginMethod public void hide(PluginCall call){getActivity().runOnUiThread(()->{
        if(!trusted(call))return;try{long sequence=BridgeArgs.integer(BridgeArgs.exact(call,"sequence"),"sequence");if(sequence<0)throw new IllegalArgumentException("INVALID_ARGUMENT");if(activity().backdropHost!=null)activity().backdropHost.hide(sequence);call.resolve();}catch(Exception e){BridgeArgs.reject(call,e);}
    });}
    @PluginMethod public void getMetrics(PluginCall call){getActivity().runOnUiThread(()->{
        if(!trusted(call))return;try{BridgeArgs.exact(call);BackdropNavHost host=activity().backdropHost;call.resolve(new JSObject().put("active",host!=null&&host.active()).put("renderer","compose-backdrop").put("captured_bitmaps",0).put("source_draws",host==null?0:host.getBackdropDraws().get()).put("nav_draws",host==null?0:host.getNavDraws().get()));}catch(Exception e){BridgeArgs.reject(call,e);}
    });}
    private float number(JSObject a,String name)throws Exception{Object v=a.get(name);if(!(v instanceof Number)||!Double.isFinite(((Number)v).doubleValue()))throw new IllegalArgumentException("INVALID_ARGUMENT");return ((Number)v).floatValue();}
    @Override protected void handleOnPause(){if(activity().backdropHost!=null)getActivity().runOnUiThread(()->activity().backdropHost.pause());}
    @Override protected void handleOnResume(){if(activity().backdropHost!=null)getActivity().runOnUiThread(()->activity().backdropHost.resume());}
}
