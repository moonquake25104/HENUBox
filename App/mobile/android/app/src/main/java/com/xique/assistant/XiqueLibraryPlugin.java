package com.xique.assistant;

import android.content.Intent;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

/** Opens one fixed official website. No credentials, cookies, URLs or page scripts cross this bridge. */
@CapacitorPlugin(name = "XiqueLibrary")
public final class XiqueLibraryPlugin extends Plugin {
    @PluginMethod public void open(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!SessionPolicy.trustedPage(getBridge().getWebView().getUrl())
                    || getBridge().getServerUrl() != null) {
                call.reject("Untrusted page", "UNTRUSTED_ORIGIN");
                return;
            }
            try {
                BridgeArgs.exact(call);
                getActivity().startActivity(new Intent(getActivity(), LibraryBrowserActivity.class));
                call.resolve();
            } catch (Exception error) {
                call.reject("Library browser could not open", "BROWSER_UNAVAILABLE");
            }
        });
    }
}
