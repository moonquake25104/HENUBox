package com.xique.assistant;

import com.getcapacitor.*;
import com.getcapacitor.annotation.CapacitorPlugin;

/** Presentation only; accepts a theme enum, never arbitrary colors or settings. */
@CapacitorPlugin(name="XiqueAppearance")
public final class XiqueAppearancePlugin extends Plugin {
    @PluginMethod public void getIntroNotice(PluginCall call) { getActivity().runOnUiThread(() -> {
        if(!SessionPolicy.trustedPage(getBridge().getWebView().getUrl())||getBridge().getServerUrl()!=null){call.reject("Untrusted page","UNTRUSTED_ORIGIN");return;}
        try{BridgeArgs.exact(call);call.resolve(new JSObject().put("acknowledged",getContext().getSharedPreferences("xique-appearance",android.content.Context.MODE_PRIVATE).getBoolean("intro_notice_v1",false)));}catch(Exception e){BridgeArgs.reject(call,e);}
    });}
    @PluginMethod public void acknowledgeIntroNotice(PluginCall call) { getActivity().runOnUiThread(() -> {
        if(!SessionPolicy.trustedPage(getBridge().getWebView().getUrl())||getBridge().getServerUrl()!=null){call.reject("Untrusted page","UNTRUSTED_ORIGIN");return;}
        try{BridgeArgs.exact(call);if(!getContext().getSharedPreferences("xique-appearance",android.content.Context.MODE_PRIVATE).edit().putBoolean("intro_notice_v1",true).commit()){call.reject("Unable to save notice","STORAGE_UNAVAILABLE");return;}call.resolve(new JSObject().put("acknowledged",true));}catch(Exception e){BridgeArgs.reject(call,e);}
    });}
    @PluginMethod public void getTimetableDays(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!SessionPolicy.trustedPage(getBridge().getWebView().getUrl()) || getBridge().getServerUrl()!=null) {call.reject("Untrusted page","UNTRUSTED_ORIGIN");return;}
            try {
                BridgeArgs.exact(call);
                int days=getContext().getSharedPreferences("xique-appearance",android.content.Context.MODE_PRIVATE).getInt("timetable_days",0);
                call.resolve(new JSObject().put("days",days==5||days==7?days:0));
            } catch(Exception e) { BridgeArgs.reject(call,e); }
        });
    }
    @PluginMethod public void setTimetableDays(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!SessionPolicy.trustedPage(getBridge().getWebView().getUrl()) || getBridge().getServerUrl()!=null) {call.reject("Untrusted page","UNTRUSTED_ORIGIN");return;}
            try {
                long days=BridgeArgs.integer(BridgeArgs.exact(call,"days"),"days");
                if(days!=5&&days!=7)throw new IllegalArgumentException("INVALID_ARGUMENT");
                if(!getContext().getSharedPreferences("xique-appearance",android.content.Context.MODE_PRIVATE).edit().putInt("timetable_days",(int)days).commit()) {call.reject("Unable to save display preference","STORAGE_UNAVAILABLE");return;}
                call.resolve();
            } catch(Exception e) { BridgeArgs.reject(call,e); }
        });
    }
    @PluginMethod public void setAppearance(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!SessionPolicy.trustedPage(getBridge().getWebView().getUrl()) || getBridge().getServerUrl()!=null) {call.reject("Untrusted page","UNTRUSTED_ORIGIN");return;}
            try {
                boolean hasPalette=call.getData().has("palette");
                boolean hasMode=call.getData().has("mode");
                JSObject args=hasPalette?(hasMode?BridgeArgs.exact(call,"theme","style","palette","mode"):BridgeArgs.exact(call,"theme","style","palette")):BridgeArgs.exact(call,"theme","style");String theme=BridgeArgs.text(args,"theme",5),style=BridgeArgs.text(args,"style",7);
                if((!theme.equals("light")&&!theme.equals("dark"))||(!style.equals("classic")&&!style.equals("glass")&&!style.equals("android")))throw new IllegalArgumentException("INVALID_ARGUMENT");
                if(hasPalette){String palette=BridgeArgs.text(args,"palette",16);if(!PresentationPalette.valid(palette))throw new IllegalArgumentException("INVALID_ARGUMENT");if(hasMode)((MainActivity)getActivity()).setAppAppearance(theme,style,palette,BridgeArgs.text(args,"mode",6));else ((MainActivity)getActivity()).setAppAppearance(theme,style,palette);}
                else ((MainActivity)getActivity()).setAppAppearance(theme,style);call.resolve();
            }catch(Exception e){BridgeArgs.reject(call,e);}
        });
    }
    @PluginMethod public void setTheme(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!SessionPolicy.trustedPage(getBridge().getWebView().getUrl()) || getBridge().getServerUrl()!=null) {
                call.reject("Untrusted page","UNTRUSTED_ORIGIN");return;
            }
            try {
                JSObject args=BridgeArgs.exact(call,"theme");
                String theme=BridgeArgs.text(args,"theme",5);
                if(!theme.equals("light") && !theme.equals("dark"))throw new IllegalArgumentException();
                ((MainActivity)getActivity()).setAppAppearance(theme);
                call.resolve();
            } catch(Exception e) { BridgeArgs.reject(call,e); }
        });
    }
}
