package com.xique.assistant;

import android.content.Context;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageManager;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import com.xique.assistant.focus.FocusWidget;
import java.util.Arrays;
import java.util.HashSet;
import java.util.Set;
import org.junit.Test;
import org.junit.runner.RunWith;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;

@RunWith(AndroidJUnit4.class)
public class WidgetsDeviceTest {
    @Test public void registersOnlySupportedWidgets() throws Exception {
        Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
        PackageManager packages = context.getPackageManager();
        ActivityInfo[] receivers = packages.getPackageInfo(context.getPackageName(),
                PackageManager.GET_RECEIVERS | PackageManager.GET_META_DATA).receivers;
        Set<String> names = new HashSet<>();
        for (ActivityInfo receiver : receivers) {
            if (receiver.metaData != null && receiver.metaData.containsKey("android.appwidget.provider")) {
                assertFalse(receiver.exported);
                names.add(receiver.name);
            }
        }
        Set<String> expected = new HashSet<>(Arrays.asList(
                FocusWidget.class.getName(), TimetableWidget.class.getName(),
                WeekTimetableWidget.FiveDays.class.getName(), WeekTimetableWidget.SevenDays.class.getName()));
        assertEquals(expected, names);
    }
}
