package com.xique.assistant;
import android.content.*;
import android.content.pm.*;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.junit.Test;
import org.junit.runner.RunWith;
import static org.junit.Assert.*;

@RunWith(AndroidJUnit4.class)
public class NoWidgetsDeviceTest {
    @Test public void noWidgetReceiversOrPinMethodsRemain() throws Exception {
        Context c=InstrumentationRegistry.getInstrumentation().getTargetContext();
        PackageInfo info=c.getPackageManager().getPackageInfo(c.getPackageName(),PackageManager.GET_RECEIVERS|PackageManager.GET_META_DATA);
        if(info.receivers!=null)for(ActivityInfo receiver:info.receivers){assertFalse(receiver.name.contains("Widget"));assertTrue(receiver.metaData==null||!receiver.metaData.containsKey("android.appwidget.provider"));}
        for(Class<?> plugin:new Class<?>[]{XiqueTodoPlugin.class,XiqueFocusExtrasPlugin.class})for(java.lang.reflect.Method method:plugin.getDeclaredMethods())assertNotEquals("pinWidget",method.getName());
        assertFalse(com.xique.assistant.focus.FocusExtras.state(c).has("widget_supported"));
    }
}
