package com.xique.assistant.focus;

import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.os.SystemClock;
import android.util.Log;
import android.view.View;
import android.widget.RemoteViews;
import com.xique.assistant.MainActivity;
import com.xique.assistant.R;
import com.xique.assistant.WidgetAppearance;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class FocusWidget extends AppWidgetProvider {
    public static final String OPEN_FOCUS = "com.xique.assistant.OPEN_FOCUS";
    private static final ExecutorService io = Executors.newSingleThreadExecutor();

    public static ComponentName component(Context context) {
        return new ComponentName(context, FocusWidget.class);
    }

    public static void refresh(Context context) {
        schedule(context, null);
    }

    @Override public void onUpdate(Context context, AppWidgetManager manager, int[] ids) {
        schedule(context, goAsync());
    }

    @Override public void onReceive(Context context, Intent intent) {
        super.onReceive(context, intent);
        if (Intent.ACTION_BOOT_COMPLETED.equals(intent.getAction()) || Intent.ACTION_MY_PACKAGE_REPLACED.equals(intent.getAction())) {
            schedule(context, goAsync());
        }
    }

    private static void schedule(Context context, PendingResult pending) {
        Context app = context.getApplicationContext();
        io.execute(() -> {
            try {
                AppWidgetManager manager = AppWidgetManager.getInstance(app);
                int[] ids = manager.getAppWidgetIds(component(app));
                if (ids.length == 0) return;
                manager.updateAppWidget(ids, views(app, FocusEngine.get(app).widgetState()));
            } catch (Exception failure) {
                Log.w("HENUBoxWidget", "Unable to refresh focus widget", failure);
            } finally {
                if (pending != null) pending.finish();
            }
        });
    }

    private static RemoteViews views(Context context, FocusEngine.WidgetState state) {
        RemoteViews views = new RemoteViews(context.getPackageName(), R.layout.widget_focus);
        WidgetAppearance.apply(context, views);
        views.setTextViewText(R.id.focus_widget_status, state.label);
        views.setViewVisibility(R.id.focus_widget_clock, state.running ? View.VISIBLE : View.GONE);
        views.setViewVisibility(R.id.focus_widget_time, state.running ? View.GONE : View.VISIBLE);
        long seconds = (state.milliseconds + 999) / 1000;
        views.setTextViewText(R.id.focus_widget_time, String.format(Locale.ROOT, "%02d:%02d", seconds / 60, seconds % 60));
        if (state.running) {
            long base = SystemClock.elapsedRealtime() + (state.countdown ? state.milliseconds : -state.milliseconds);
            views.setChronometerCountDown(R.id.focus_widget_clock, state.countdown);
            views.setChronometer(R.id.focus_widget_clock, base, null, true);
        }
        Intent open = new Intent(context, MainActivity.class).setAction(OPEN_FOCUS)
                .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        views.setOnClickPendingIntent(R.id.focus_widget_root,
                PendingIntent.getActivity(context, 7310, open, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT));
        return views;
    }
}
