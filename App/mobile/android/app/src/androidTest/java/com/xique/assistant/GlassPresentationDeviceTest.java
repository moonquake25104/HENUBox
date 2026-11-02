package com.xique.assistant;

import android.graphics.*;
import androidx.core.view.*;
import androidx.test.core.app.ActivityScenario;
import androidx.test.platform.app.InstrumentationRegistry;
import org.json.*;
import org.junit.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicReference;
import static org.junit.Assert.*;

/** In-memory pixel/appearance checks only. Never saves personal course screenshots. */
public class GlassPresentationDeviceTest {
    MainActivity activity;
    private JSONObject js(String expression)throws Exception{
        CountDownLatch done=new CountDownLatch(1);AtomicReference<String> raw=new AtomicReference<>();
        InstrumentationRegistry.getInstrumentation().runOnMainSync(()->activity.getBridge().getWebView().evaluateJavascript("JSON.stringify(("+expression+"))",v->{raw.set(v);done.countDown();}));
        assertTrue(done.await(10,TimeUnit.SECONDS));Object value=new JSONTokener(raw.get()).nextValue();assertTrue(value instanceof String);return new JSONObject((String)value);
    }
    private void waitFor(String condition)throws Exception{long end=System.nanoTime()+TimeUnit.SECONDS.toNanos(15);while(System.nanoTime()<end){try{if(js("({ready:!!("+condition+")})").getBoolean("ready"))return;}catch(Exception|AssertionError ignored){}Thread.sleep(100);}fail("UI condition timeout");}
    private void appearance(String theme,String style)throws Exception{
        js("(()=>{document.documentElement.classList.toggle('dark',"+theme.equals("dark")+");document.documentElement.dataset.uiStyle='"+style+"';localStorage.setItem('kairos-theme','"+theme+"');localStorage.setItem('xique-ui-style-v1','"+style+"');return {ok:true}})()");
        long end=System.nanoTime()+TimeUnit.SECONDS.toNanos(5);while(System.nanoTime()<end){android.content.SharedPreferences p=activity.getSharedPreferences("xique-appearance",0);if(theme.equals(p.getString("theme",""))&&style.equals(p.getString("style","")))return;Thread.sleep(100);}fail("Native theme/style did not synchronize");
    }
    private void pixels(String theme,String style)throws Exception{
        Thread.sleep(250);boolean dark=theme.equals("dark"),glass=style.equals("glass");int expected=Color.parseColor(glass?(dark?"#1B2C33":"#E9F2F3"):(dark?"#18191C":"#FFFFFF"));
        AtomicReference<Integer> inset=new AtomicReference<>(0);InstrumentationRegistry.getInstrumentation().runOnMainSync(()->{
            assertEquals(!dark,WindowCompat.getInsetsController(activity.getWindow(),activity.getWindow().getDecorView()).isAppearanceLightStatusBars());
            WindowInsetsCompat i=ViewCompat.getRootWindowInsets(activity.getWindow().getDecorView());if(i!=null)inset.set(i.getInsets(WindowInsetsCompat.Type.statusBars()).top);
        });
        assertTrue("Status inset unavailable",inset.get()>5);Bitmap screen=InstrumentationRegistry.getInstrumentation().getUiAutomation().takeScreenshot();assertNotNull(screen);
        try{
            int matched=0,total=0;for(int y=2;y<inset.get()-2;y+=4)for(int x=4;x<screen.getWidth()-4;x+=5){total++;if(close(screen.getPixel(x,y),expected))matched++;}
            assertTrue("Visible status-bar pixels differ: "+theme+"/"+style+" expected="+Integer.toHexString(expected)+" sample="+Integer.toHexString(screen.getPixel(screen.getWidth()/2,3))+" ratio="+matched+"/"+total,matched>total*.65);
            if(glass){int seam=0,denom=0;for(int x=5;x<screen.getWidth()-5;x+=5){denom++;if(close(screen.getPixel(x,inset.get()-3),expected)&&close(screen.getPixel(x,inset.get()+6),expected))seam++;}assertTrue("Native/WebView seam remains",seam>denom*.9);}
        }finally{screen.recycle();}
    }
    private boolean close(int a,int b){return Math.abs(Color.red(a)-Color.red(b))<5&&Math.abs(Color.green(a)-Color.green(b))<5&&Math.abs(Color.blue(a)-Color.blue(b))<5;}
    @Test public void glassBarsAndLifecycleStayInSync()throws Exception{
        Assume.assumeTrue(android.os.Build.HARDWARE.contains("ranchu")||android.os.Build.HARDWARE.contains("goldfish"));
        try(ActivityScenario<MainActivity> scenario=ActivityScenario.launch(MainActivity.class)){
            scenario.onActivity(a->activity=a);waitFor("document.querySelector('.xm-navigation')");JSONObject original=js("({theme:document.documentElement.classList.contains('dark')?'dark':'light',style:document.documentElement.dataset.uiStyle||'classic',hash:location.hash})");
            try{
                js("(()=>{location.hash='/home';return {ok:true}})()");
                for(String[] pair:new String[][]{{"light","classic"},{"light","glass"},{"dark","glass"},{"dark","classic"}}){appearance(pair[0],pair[1]);pixels(pair[0],pair[1]);}
                appearance("dark","glass");scenario.recreate();scenario.onActivity(a->activity=a);waitFor("document.querySelector('.xm-navigation')");pixels("dark","glass");
                js("(()=>{location.hash='/campus';return {ok:true}})()");waitFor("document.querySelectorAll('.xm-campus-grid button').length===6");
                assertTrue(js("({ok:[...document.querySelectorAll('.xm-campus-grid button')].every(e=>{const c=getComputedStyle(e),r=e.getBoundingClientRect();return c.backdropFilter==='none'&&r.width>0&&r.bottom<document.querySelector('.xm-navigation').getBoundingClientRect().top})})").getBoolean("ok"));
                System.out.println("GLASS_PRESENTATION_OK four_material_theme_combinations=true native_web_seam_pixels=true recreate=true six_cards=true private_screenshot_saved=false");
            }finally{appearance(original.getString("theme"),original.getString("style"));js("(()=>{location.hash="+JSONObject.quote(original.getString("hash"))+";return {ok:true}})()");}
        }
    }
}
