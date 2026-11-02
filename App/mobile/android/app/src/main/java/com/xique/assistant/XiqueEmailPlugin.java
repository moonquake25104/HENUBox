package com.xique.assistant;

import android.content.Intent;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

@CapacitorPlugin(name="XiqueEmail")
public final class XiqueEmailPlugin extends Plugin {
    @PluginMethod public void open(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!SessionPolicy.trustedPage(getBridge().getWebView().getUrl()) || getBridge().getServerUrl()!=null) {
                call.reject("Untrusted page","UNTRUSTED_ORIGIN"); return;
            }
            try {
                BridgeArgs.exact(call);
                getActivity().startActivity(new Intent(getActivity(),EmailBrowserActivity.class));
                call.resolve();
            } catch(Exception e) { call.reject("Email browser could not open","BROWSER_UNAVAILABLE"); }
        });
    }
}
