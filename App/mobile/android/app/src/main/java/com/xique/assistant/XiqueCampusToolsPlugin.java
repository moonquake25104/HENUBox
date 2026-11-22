package com.xique.assistant;

import android.appwidget.AppWidgetManager;
import android.content.ActivityNotFoundException;
import android.content.ComponentName;
import android.content.Intent;
import android.net.Uri;
import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.xique.assistant.focus.FocusWidget;
import java.net.URI;

@CapacitorPlugin(name = "XiqueCampusTools")
public final class XiqueCampusToolsPlugin extends Plugin {
    private void fromApp(PluginCall call, Action action) {
        getActivity().runOnUiThread(() -> {
            if (!SessionPolicy.trustedPage(getBridge().getWebView().getUrl()) || getBridge().getServerUrl() != null) {
                call.reject("Untrusted page", "UNTRUSTED_ORIGIN");
                return;
            }
            try {
                action.run();
            } catch (ActivityNotFoundException e) {
                call.reject("No app can complete this action", "APP_UNAVAILABLE");
            } catch (IllegalArgumentException | org.json.JSONException e) {
                call.reject("Invalid arguments", "INVALID_ARGUMENT");
            }
        });
    }

    @FunctionalInterface
    private interface Action {
        void run() throws org.json.JSONException;
    }

    @PluginMethod public void share(PluginCall call) {
        fromApp(call, () -> {
            String text = BridgeArgs.text(BridgeArgs.exact(call, "text"), "text", 3520);
            Intent message = new Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, text);
            getActivity().startActivity(Intent.createChooser(message, null));
            call.resolve();
        });
    }

    @PluginMethod public void openLink(PluginCall call) {
        fromApp(call, () -> {
            URI uri = URI.create(BridgeArgs.text(BridgeArgs.exact(call, "url"), "url", 2048));
            if (!"https".equalsIgnoreCase(uri.getScheme()) || uri.getHost() == null || uri.getRawUserInfo() != null) {
                throw new IllegalArgumentException();
            }
            Intent link = new Intent(Intent.ACTION_VIEW, Uri.parse(uri.toASCIIString())).addCategory(Intent.CATEGORY_BROWSABLE);
            getActivity().startActivity(link);
            call.resolve();
        });
    }

    @PluginMethod public void pinFocus(PluginCall call) {
        pinWidget(call, FocusWidget.component(getContext()));
    }

    @PluginMethod public void pinTimetable(PluginCall call) {
        pinWidget(call, TimetableWidget.component(getContext()));
    }

    @PluginMethod public void pinWeekTimetable(PluginCall call) {
        fromApp(call, () -> {
            long days = BridgeArgs.integer(BridgeArgs.exact(call, "days"), "days");
            if (days != 5 && days != 7) throw new IllegalArgumentException();
            resolvePin(call, WeekTimetableWidget.component(getContext(), (int) days));
        });
    }

    @PluginMethod public void getWidgetOpacity(PluginCall call) {
        fromApp(call, () -> {
            BridgeArgs.exact(call);
            call.resolve(new JSObject().put("opacity", WidgetAppearance.opacity(getContext())));
        });
    }

    @PluginMethod public void setWidgetOpacity(PluginCall call) {
        fromApp(call, () -> {
            WidgetAppearance.setOpacity(getContext(), BridgeArgs.integer(BridgeArgs.exact(call, "opacity"), "opacity"));
            FocusWidget.refresh(getContext());
            TimetableWidget.refresh(getContext());
            WeekTimetableWidget.refresh(getContext());
            call.resolve();
        });
    }

    private void pinWidget(PluginCall call, ComponentName provider) {
        fromApp(call, () -> {
            BridgeArgs.exact(call);
            resolvePin(call, provider);
        });
    }

    private void resolvePin(PluginCall call, ComponentName provider) {
        AppWidgetManager manager = AppWidgetManager.getInstance(getContext());
        boolean requested = false;
        try {
            requested = manager.isRequestPinAppWidgetSupported()
                    && manager.requestPinAppWidget(provider, null, null);
        } catch (IllegalStateException | SecurityException ignored) {
            // The user can still add from the launcher's widget picker.
        }
        call.resolve(new JSObject().put("requested", requested));
    }
}
