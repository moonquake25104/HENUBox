package com.xique.assistant;

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
import android.view.View;
import android.widget.RemoteViews;
import com.xique.assistant.WidgetTimetableData.Lesson;
import java.time.DayOfWeek;
import java.time.LocalDate;
import java.time.LocalTime;
import java.time.temporal.TemporalAdjusters;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import org.json.JSONArray;

public abstract class WeekTimetableWidget extends AppWidgetProvider {
    private static final ExecutorService IO = Executors.newSingleThreadExecutor();
    private static final String[] DAYS = {"周一", "周二", "周三", "周四", "周五", "周六", "周日"};
    private static final int MIN_WIDTH = 224;
    private static final int COMPACT_HEIGHT = 188;
    private static final int REGULAR_HEIGHT = 288;
    private static final int TALL_HEIGHT = 392;

    public static final class FiveDays extends WeekTimetableWidget {
        @Override int days() { return 5; }
    }

    public static final class SevenDays extends WeekTimetableWidget {
        @Override int days() { return 7; }
    }

    abstract int days();

    static ComponentName component(Context context, int days) {
        return new ComponentName(context, days == 5 ? FiveDays.class : SevenDays.class);
    }

    static void refresh(Context context) {
        queue(context, 0, null);
    }

    @Override public void onUpdate(Context context, AppWidgetManager manager, int[] ids) {
        queue(context, days(), goAsync());
    }

    @Override public void onAppWidgetOptionsChanged(Context context, AppWidgetManager manager, int id, Bundle options) {
        queue(context, days(), goAsync());
    }

    @Override public void onReceive(Context context, Intent intent) {
        super.onReceive(context, intent);
        String action = intent.getAction();
        if (Intent.ACTION_BOOT_COMPLETED.equals(action) || Intent.ACTION_MY_PACKAGE_REPLACED.equals(action)
                || Intent.ACTION_TIME_CHANGED.equals(action) || Intent.ACTION_TIMEZONE_CHANGED.equals(action)) {
            queue(context, days(), goAsync());
        }
    }

    private static void queue(Context context, int days, PendingResult pending) {
        Context app = context.getApplicationContext();
        IO.execute(() -> {
            try {
                AppWidgetManager manager = AppWidgetManager.getInstance(app);
                int[] weekdays = days == 7 ? new int[0] : manager.getAppWidgetIds(component(app, 5));
                int[] fullWeek = days == 5 ? new int[0] : manager.getAppWidgetIds(component(app, 7));
                if (weekdays.length == 0 && fullWeek.length == 0) return;
                JSONArray courses = LocalCore.service(app).bootstrap().getJSONArray("courses");
                LocalDate today = LocalDate.now(CourseReminderPlanner.ZONE);
                LocalDate monday = today.with(TemporalAdjusters.previousOrSame(DayOfWeek.MONDAY));
                List<List<Lesson>> schedule = new ArrayList<>();
                int daysToLoad = fullWeek.length == 0 ? 5 : 7;
                for (int i = 0; i < daysToLoad; i++) {
                    schedule.add(WidgetTimetableData.lessons(courses, monday.plusDays(i), LocalTime.MIN));
                }
                Intent open = new Intent(app, MainActivity.class).setAction(CourseReminders.OPEN)
                        .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP | Intent.FLAG_ACTIVITY_SINGLE_TOP);
                PendingIntent click = PendingIntent.getActivity(app, 7322, open,
                        PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
                update(app, manager, weekdays, 5, today, monday, schedule, courses.length() == 0, click);
                update(app, manager, fullWeek, 7, today, monday, schedule, courses.length() == 0, click);
            } catch (Exception error) {
                Log.w("HENUBoxTimetable", "Unable to refresh week timetable widget", error);
            } finally {
                if (pending != null) pending.finish();
            }
        });
    }

    private static void update(Context context, AppWidgetManager manager, int[] ids, int days, LocalDate today,
                               LocalDate monday, List<List<Lesson>> schedule, boolean emptyStore, PendingIntent click) {
        if (ids.length == 0) return;
        Week week = new Week(today, monday, schedule.subList(0, days), emptyStore);
        RemoteViews compact = views(context, week, 2, click);
        RemoteViews regular = views(context, week, 4, click);
        RemoteViews tall = views(context, week, 6, click);
        if (Build.VERSION.SDK_INT >= 31) {
            Map<SizeF, RemoteViews> layouts = new LinkedHashMap<>();
            layouts.put(new SizeF(MIN_WIDTH, COMPACT_HEIGHT), compact);
            layouts.put(new SizeF(MIN_WIDTH, REGULAR_HEIGHT), regular);
            layouts.put(new SizeF(MIN_WIDTH, TALL_HEIGHT), tall);
            manager.updateAppWidget(ids, new RemoteViews(layouts));
            return;
        }
        for (int id : ids) {
            int height = manager.getAppWidgetOptions(id).getInt(AppWidgetManager.OPTION_APPWIDGET_MIN_HEIGHT, REGULAR_HEIGHT);
            manager.updateAppWidget(id, height >= TALL_HEIGHT ? tall : height >= REGULAR_HEIGHT ? regular : compact);
        }
    }

    private static RemoteViews views(Context context, Week week, int limit, PendingIntent click) {
        RemoteViews views = new RemoteViews(context.getPackageName(), R.layout.widget_week_timetable);
        WidgetAppearance.apply(context, views);
        views.setOnClickPendingIntent(R.id.week_widget_root, click);
        views.setTextViewText(R.id.week_widget_range, context.getString(R.string.widget_week_range,
                dateLabel(week.monday), dateLabel(week.monday.plusDays(week.schedule.size() - 1))));
        views.removeAllViews(R.id.week_widget_columns);
        for (int i = 0; i < week.schedule.size(); i++) {
            LocalDate date = week.monday.plusDays(i);
            RemoteViews column = new RemoteViews(context.getPackageName(), R.layout.widget_week_column);
            column.setTextViewText(R.id.week_widget_day, DAYS[i]);
            column.setTextViewText(R.id.week_widget_date, dateLabel(date));
            column.setViewVisibility(R.id.week_widget_today, date.equals(week.today) ? View.VISIBLE : View.GONE);
            addLessons(context, column, week.schedule.get(i), limit);
            views.addView(R.id.week_widget_columns, column);
        }
        boolean hasLessons = week.schedule.stream().anyMatch(day -> !day.isEmpty());
        views.setViewVisibility(R.id.week_widget_empty, hasLessons ? View.GONE : View.VISIBLE);
        views.setTextViewText(R.id.week_widget_empty, context.getString(
                week.emptyStore ? R.string.widget_week_import : R.string.widget_week_empty));
        return views;
    }

    private static void addLessons(Context context, RemoteViews column, List<Lesson> lessons, int limit) {
        column.removeAllViews(R.id.week_widget_lessons);
        int shown = Math.min(lessons.size(), limit);
        for (int index = 0; index < shown; index++) {
            Lesson lesson = lessons.get(index);
            RemoteViews card = new RemoteViews(context.getPackageName(), R.layout.widget_week_lesson);
            card.setTextViewText(R.id.week_widget_lesson_name, lesson.name);
            card.setTextViewText(R.id.week_widget_lesson_start, lesson.start.toString());
            column.addView(R.id.week_widget_lessons, card);
        }
        int more = lessons.size() - shown;
        column.setViewVisibility(R.id.week_widget_more, more > 0 ? View.VISIBLE : View.GONE);
        if (more > 0) {
            column.setTextViewText(R.id.week_widget_more, context.getString(R.string.widget_week_more, more));
            column.setContentDescription(R.id.week_widget_more, context.getString(R.string.widget_week_more_description, more));
        }
    }

    private static String dateLabel(LocalDate date) {
        return date.getMonthValue() + "/" + date.getDayOfMonth();
    }

    private static final class Week {
        final LocalDate today;
        final LocalDate monday;
        final List<List<Lesson>> schedule;
        final boolean emptyStore;

        Week(LocalDate today, LocalDate monday, List<List<Lesson>> schedule, boolean emptyStore) {
            this.today = today;
            this.monday = monday;
            this.schedule = schedule;
            this.emptyStore = emptyStore;
        }
    }
}
