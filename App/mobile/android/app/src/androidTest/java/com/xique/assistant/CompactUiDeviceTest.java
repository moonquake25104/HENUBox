package com.xique.assistant;

import android.graphics.Bitmap;
import android.graphics.Color;
import android.graphics.drawable.ColorDrawable;
import androidx.core.view.WindowCompat;
import androidx.test.core.app.ActivityScenario;
import androidx.test.platform.app.InstrumentationRegistry;
import org.json.*;
import org.junit.Test;
import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicReference;
import static org.junit.Assert.*;

/** Geometry/appearance only. No course text, credentials, screenshots or DB copies saved. */
public class CompactUiDeviceTest {
    private MainActivity activity;
    private JSONObject js(String expression) throws Exception {
        CountDownLatch done=new CountDownLatch(1);AtomicReference<String> raw=new AtomicReference<>();
        InstrumentationRegistry.getInstrumentation().runOnMainSync(()->activity.getBridge().getWebView()
            .evaluateJavascript("JSON.stringify(("+expression+"))",value->{raw.set(value);done.countDown();}));
        assertTrue("WebView evaluation timed out",done.await(10,TimeUnit.SECONDS));
        Object value=new JSONTokener(raw.get()).nextValue();
        if(!(value instanceof String))throw new AssertionError("UI state not ready");
        return new JSONObject((String)value);
    }
    private void waitFor(String condition) throws Exception {
        long end=System.nanoTime()+TimeUnit.SECONDS.toNanos(15);
        while(System.nanoTime()<end) {try { if(js("({ready:!!("+condition+")})").getBoolean("ready"))return; } catch(AssertionError ignored) {}Thread.sleep(100);}
        fail("UI state did not become ready");
    }
    private void theme(boolean dark) throws Exception {
        js("(()=>{document.documentElement.classList.toggle('dark',"+dark+");localStorage.setItem('kairos-theme','"+(dark?"dark":"light")+"');window.dispatchEvent(new Event('theme-change'));return {changed:true}})()");
        waitFor("window.Capacitor?.Plugins?.XiqueAppearance");
        long end=System.nanoTime()+TimeUnit.SECONDS.toNanos(5);
        while(System.nanoTime()<end) {
            if((dark?"dark":"light").equals(activity.getSharedPreferences("xique-appearance",0).getString("theme","")))return;
            Thread.sleep(100);
        }
        fail("Native appearance was not synchronized");
    }
    @Test public void compactPagesAndNativeBars() throws Exception {
        assertEquals("true",InstrumentationRegistry.getArguments().getString("run_ui_checks"));
        try(ActivityScenario<MainActivity> scenario=ActivityScenario.launch(MainActivity.class)) {
            scenario.onActivity(value->activity=value);
            waitFor("document.querySelector('.xm-navigation')");
            boolean original=js("({dark:document.documentElement.classList.contains('dark')})").getBoolean("dark");
            try {
                js("(()=>{location.hash='/campus';return {changed:true}})()");waitFor("document.querySelectorAll('.xm-campus-grid button').length===4");
                JSONObject campus=js("(()=>{const n=document.querySelector('.xm-navigation').getBoundingClientRect();const c=[...document.querySelectorAll('.xm-campus-grid button')].map(x=>x.getBoundingClientRect());return {count:c.length,fit:c.every(x=>x.top>=0&&x.bottom<n.top-12),noScroll:document.documentElement.scrollHeight<=innerHeight+1&&document.documentElement.scrollWidth<=innerWidth+1}})()");
                assertEquals(4,campus.getInt("count"));assertTrue(campus.getBoolean("fit"));assertTrue(campus.getBoolean("noScroll"));
                js("(()=>{location.hash='/timetable';return {changed:true}})()");waitFor("document.querySelector('.xm-week-body')");
                waitFor("document.querySelectorAll('.xm-period-time').length===13");
                assertTrue(js("({matches:document.querySelector('.xm-period-time').dataset.start==='08:30'&&document.querySelector('.xm-period-time').dataset.end==='09:15'})").getBoolean("matches"));
                JSONObject week=js("(()=>{const g=document.querySelector('.xm-week-body'),r=g.getBoundingClientRect(),n=document.querySelector('.xm-navigation').getBoundingClientRect();return {days:document.querySelectorAll('.xm-week-head>div').length,fit:r.bottom<n.top-4,noScroll:document.documentElement.scrollHeight<=innerHeight+1&&document.documentElement.scrollWidth<=innerWidth+1,internalScroll:g.scrollWidth>g.clientWidth+1||g.scrollHeight>g.clientHeight+1}})()");
                assertEquals(7,week.getInt("days"));assertTrue(week.getBoolean("fit"));assertTrue(week.getBoolean("noScroll"));assertFalse(week.getBoolean("internalScroll"));
                for(boolean dark:new boolean[]{true,false}) {
                    theme(dark);Thread.sleep(250);
                    AtomicReference<Boolean> barsMatch=new AtomicReference<>(false);
                    InstrumentationRegistry.getInstrumentation().runOnMainSync(()->{
                        int color=dark?0xff18191c:Color.WHITE;
                        ColorDrawable host=(ColorDrawable)activity.findViewById(android.R.id.content).getBackground();
                        barsMatch.set(host.getColor()==color && WindowCompat.getInsetsController(activity.getWindow(),activity.getWindow().getDecorView()).isAppearanceLightStatusBars()==!dark);
                    });assertTrue("Native host and status icon theme mismatch",barsMatch.get());
                    Bitmap screenshot=InstrumentationRegistry.getInstrumentation().getUiAutomation().takeScreenshot();
                    assertNotNull("Screenshot unavailable for status-bar color sampling",screenshot);
                    try {
                        int expected=dark?0xff18191c:Color.WHITE,matched=0,total=0;
                        int height=Math.min(screenshot.getHeight(),Math.max(12,(int)(24*activity.getResources().getDisplayMetrics().density)));
                        for(int y=2;y<height;y+=4)for(int x=4;x<screenshot.getWidth();x+=4){total++;int c=screenshot.getPixel(x,y);if(Math.abs(Color.red(c)-Color.red(expected))<8&&Math.abs(Color.green(c)-Color.green(expected))<8&&Math.abs(Color.blue(c)-Color.blue(expected))<8)matched++;}
                        assertTrue("Visible status bar did not match theme",matched>total*.65);
                    } finally {screenshot.recycle();}
                }
                System.out.println("COMPACT_UI_OK seven_days=true four_cards=true light_dark_status_bar_pixels=true private_screenshot_saved=false");
            } finally {theme(original);js("(()=>{location.hash='/timetable';return {restored:true}})()");}
        }
    }
}
