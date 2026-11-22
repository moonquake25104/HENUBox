package com.xique.assistant;

import android.content.Context;
import android.content.SharedPreferences;
import android.widget.RemoteViews;

public final class WidgetAppearance {
    private static final String KEY = "opacity";

    private WidgetAppearance() {}

    private static SharedPreferences preferences(Context context) {
        return context.getSharedPreferences("widget_appearance", Context.MODE_PRIVATE);
    }

    static int opacity(Context context) {
        return preferences(context).getInt(KEY, 80);
    }

    static void setOpacity(Context context, long value) {
        if (value != 65 && value != 80 && value != 100) throw new IllegalArgumentException();
        preferences(context).edit().putInt(KEY, (int) value).apply();
    }

    public static void apply(Context context, RemoteViews views) {
        int value = opacity(context);
        int surface = value == 100 ? R.drawable.widget_focus_surface
                : value == 65 ? R.drawable.widget_surface_clear : R.drawable.widget_surface_soft;
        views.setImageViewResource(R.id.widget_surface, surface);
    }
}
