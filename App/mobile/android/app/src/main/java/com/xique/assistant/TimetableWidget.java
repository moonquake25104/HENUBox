package com.xique.assistant;

import android.app.AlarmManager;
import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.os.Build;
import android.os.Bundle;
import android.util.Log;
import android.util.SizeF;
import android.widget.RemoteViews;
import java.time.LocalDate;
import java.time.LocalTime;
import java.time.ZonedDateTime;
import com.xique.assistant.WidgetTimetableData.Lesson;
import java.util.List;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import org.json.JSONArray;

public final class TimetableWidget extends AppWidgetProvider {
    private static final String REFRESH = "com.xique.assistant.TIMETABLE_WIDGET_REFRESH";
    private static final int ALARM_ID = 7321;
    private static final ExecutorService IO = Executors.newSingleThreadExecutor();

    static ComponentName component(Context context) {
        return new ComponentName(context, TimetableWidget.class);
    }

    public static void refresh(Context context) {
        queue(context, null);
    }

    @Override public void onUpdate(Context context, AppWidgetManager manager, int[] ids) {
        queue(context, goAsync());
    }

    @Override public void onAppWidgetOptionsChanged(Context context, AppWidgetManager manager, int id, Bundle options) {
        queue(context, goAsync());
    }

    @Override public void onDisabled(Context context) {
        context.getSystemService(AlarmManager.class).cancel(alarm(context));
    }

    @Override public void onReceive(Context context, Intent intent) {
        super.onReceive(context, intent);
        String action = intent.getAction();
        if (REFRESH.equals(action) || Intent.ACTION_BOOT_COMPLETED.equals(action)
                || Intent.ACTION_TIME_CHANGED.equals(action) || Intent.ACTION_TIMEZONE_CHANGED.equals(action)
                || Intent.ACTION_MY_PACKAGE_REPLACED.equals(action)) queue(context, goAsync());
    }

    private static void queue(Context context, PendingResult pending) {
        Context app = context.getApplicationContext();
        IO.execute(() -> {
            try {
                update(app);
            } catch (Exception error) {
                Log.w("HENUBoxTimetable", "Unable to refresh timetable widget", error);
            } finally {
                if (pending != null) pending.finish();
            }
        });
    }

    private static PendingIntent alarm(Context context) {
        return PendingIntent.getBroadcast(context, ALARM_ID,
                new Intent(context, TimetableWidget.class).setAction(REFRESH),
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    }

    private static void update(Context context) throws Exception {
        AppWidgetManager manager = AppWidgetManager.getInstance(context);
        int[] ids = manager.getAppWidgetIds(component(context));
        if (ids.length == 0) return;
        ZonedDateTime now = ZonedDateTime.now(CourseReminderPlanner.ZONE);
        LocalDate today = now.toLocalDate(), tomorrow = today.plusDays(1);
        JSONArray courses = LocalCore.service(context).bootstrap().getJSONArray("courses");
        List<Lesson> current = WidgetTimetableData.lessons(courses, today, now.toLocalTime());
        List<Lesson> upcoming = WidgetTimetableData.lessons(courses, tomorrow, LocalTime.MIN);
        Intent open = new Intent(context, MainActivity.class).setAction(CourseReminders.OPEN)
                .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        PendingIntent click = PendingIntent.getActivity(context, ALARM_ID, open,
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
        for (int id : ids) {
            boolean empty = courses.length() == 0;
            Bundle options = manager.getAppWidgetOptions(id);
            RemoteViews view;
            if (Build.VERSION.SDK_INT >= 31) {
                // The launcher selects a layout matching its current allocated size.
                Map<SizeF, RemoteViews> layouts = new LinkedHashMap<>();
                layouts.put(new SizeF(200, 90), view(context, click, today, current, upcoming, 1, empty));
                layouts.put(new SizeF(200, 130), view(context, click, today, current, upcoming, 2, empty));
                layouts.put(new SizeF(200, 172), view(context, click, today, current, upcoming, 3, empty));
                view = new RemoteViews(layouts);
            } else {
                int height = options.getInt(AppWidgetManager.OPTION_APPWIDGET_MIN_HEIGHT, 125);
                view = view(context, click, today, current, upcoming, height >= 130 ? 2 : 1, empty);
            }
            manager.updateAppWidget(id, view);
        }
        ZonedDateTime boundary = tomorrow.atStartOfDay(CourseReminderPlanner.ZONE);
        for (Lesson lesson : current) {
            ZonedDateTime end = today.atTime(lesson.end).atZone(CourseReminderPlanner.ZONE);
            if (end.isBefore(boundary)) boundary = end;
        }
        context.getSystemService(AlarmManager.class).set(AlarmManager.RTC, boundary.toInstant().toEpochMilli(), alarm(context));
    }

    private static RemoteViews view(Context context, PendingIntent click, LocalDate today, List<Lesson> current,
                                    List<Lesson> upcoming, int limit, boolean empty) {
        RemoteViews views = new RemoteViews(context.getPackageName(), R.layout.widget_timetable);
        WidgetAppearance.apply(context, views);
        views.setOnClickPendingIntent(R.id.timetable_widget_root, click);
        column(context, views, R.id.timetable_today, R.id.timetable_today_header, "今天", today, current, limit, empty);
        column(context, views, R.id.timetable_tomorrow, R.id.timetable_tomorrow_header, "明天", today.plusDays(1), upcoming, limit, empty);
        return views;
    }

    private static void column(Context context, RemoteViews view, int id, int header, String label,
                               LocalDate date, List<Lesson> lessons, int maximum, boolean emptyStore) {
        view.setTextViewText(header, label + " " + date.getMonthValue() + "/" + date.getDayOfMonth());
        view.removeAllViews(id);
        int shown = Math.min(maximum, lessons.size());
        for (int index = 0; index < shown; index++) {
            Lesson lesson = lessons.get(index);
            RemoteViews row = new RemoteViews(context.getPackageName(), R.layout.widget_timetable_row);
            row.setTextViewText(R.id.timetable_lesson_name, lesson.name);
            row.setTextViewText(R.id.timetable_lesson_time, lesson.start + "–" + lesson.end);
            row.setTextViewText(R.id.timetable_lesson_room, lesson.room);
            view.addView(id, row);
        }
        if (lessons.isEmpty() || shown < lessons.size()) {
            RemoteViews summary = new RemoteViews(context.getPackageName(), R.layout.widget_timetable_summary);
            summary.setTextViewText(R.id.timetable_column_summary, emptyStore ? "打开应用导入课表"
                    : lessons.isEmpty() ? "暂无课程" : "另有 " + (lessons.size() - shown) + " 节");
            view.addView(id, summary);
        }
    }

}
