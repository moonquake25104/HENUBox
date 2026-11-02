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

/** Only isolated emulator presentation checks. Screen pixels stay in memory. */
public class SettingsPaletteDeviceTest {
    MainActivity activity;
    JSONObject js(String expression)throws Exception{CountDownLatch done=new CountDownLatch(1);AtomicReference<String> raw=new AtomicReference<>();InstrumentationRegistry.getInstrumentation().runOnMainSync(()->activity.getBridge().getWebView().evaluateJavascript("JSON.stringify(("+expression+"))",v->{raw.set(v);done.countDown();}));assertTrue(done.await(10,TimeUnit.SECONDS));return new JSONObject((String)new JSONTokener(raw.get()).nextValue());}
    void waitFor(String condition)throws Exception{long end=System.nanoTime()+TimeUnit.SECONDS.toNanos(15);while(System.nanoTime()<end){try{if(js("({ok:!!("+condition+")})").getBoolean("ok"))return;}catch(Exception|AssertionError ignored){}Thread.sleep(100);}fail("Settings condition timeout");}
    void click(String selector)throws Exception{js("(()=>{document.querySelector("+JSONObject.quote(selector)+").click();return {ok:true}})()");}
    void choose(String group,String label)throws Exception{js("(()=>{[...document.querySelectorAll('[role=group][aria-label="+group+"] button')].find(e=>e.textContent==="+JSONObject.quote(label)+").click();return {ok:true}})()");}
    void synced(String id,String theme,String style)throws Exception{long end=System.nanoTime()+TimeUnit.SECONDS.toNanos(8);while(System.nanoTime()<end){android.content.SharedPreferences p=activity.getSharedPreferences("xique-appearance",0);if(id.equals(p.getString("palette",""))&&theme.equals(p.getString("theme",""))&&style.equals(p.getString("style","")))return;Thread.sleep(70);}fail("Palette did not reach native appearance");}
    String courseSignature()throws Exception{js("(()=>{window.__courseSig=null;Capacitor.Plugins.XiqueLocal.bootstrap().then(async s=>{const bytes=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(JSON.stringify(s.courses)));window.__courseSig=s.courses.length+':'+s.revision+':'+[...new Uint8Array(bytes)].join(',')});return{ok:true}})()");waitFor("window.__courseSig");return js("({signature:window.__courseSig})").getString("signature");}
    void pixels(boolean dark,boolean glass)throws Exception{
        Thread.sleep(220);JSONObject css=js("(()=>{const root=getComputedStyle(document.documentElement),body=getComputedStyle(document.body),v="+glass+"?root.getPropertyValue('--xm-glass-top'):(body.getPropertyValue('--xq-base')||body.backgroundColor),e=document.createElement('i');e.style.backgroundColor=v;document.body.append(e);const rgb=getComputedStyle(e).backgroundColor.match(/\\d+/g).slice(0,3).map(Number);e.remove();return{r:rgb[0],g:rgb[1],b:rgb[2]}})()");int expected=Color.rgb(css.getInt("r"),css.getInt("g"),css.getInt("b"));AtomicReference<Integer> top=new AtomicReference<>(0);
        InstrumentationRegistry.getInstrumentation().runOnMainSync(()->{assertEquals(!dark,WindowCompat.getInsetsController(activity.getWindow(),activity.getWindow().getDecorView()).isAppearanceLightStatusBars());top.set(ViewCompat.getRootWindowInsets(activity.getWindow().getDecorView()).getInsets(WindowInsetsCompat.Type.statusBars()).top);});assertTrue(top.get()>5);Bitmap screen=InstrumentationRegistry.getInstrumentation().getUiAutomation().takeScreenshot();try{int n=0,total=0;for(int y=2;y<top.get()-2;y+=4)for(int x=5;x<screen.getWidth()-5;x+=7){int c=screen.getPixel(x,y);if(Math.abs(Color.red(c)-Color.red(expected))<5&&Math.abs(Color.green(c)-Color.green(expected))<5&&Math.abs(Color.blue(c)-Color.blue(expected))<5)n++;total++;}assertTrue("Native status bar does not match CSS",n>total*.65);}finally{screen.recycle();}
    }
    @Test public void allPalettesAndMaterialsPersistAndMatchBars()throws Exception{
        Assume.assumeTrue(android.os.Build.HARDWARE.contains("ranchu")||android.os.Build.HARDWARE.contains("goldfish"));
        try(ActivityScenario<MainActivity> scenario=ActivityScenario.launch(MainActivity.class)){
            scenario.onActivity(a->activity=a);waitFor("document.querySelector('.xm-navigation')");js("(()=>{location.hash='/me';return{ok:true}})()");waitFor("document.querySelector('[aria-label=通用]')");click("button[aria-label=通用]");waitFor("document.querySelector('.xp-general')");
            String before=courseSignature();String[][] palettes={{"original","原色"},{"pine","松间青"},{"sea","雾海蓝"},{"lilac","云藤紫"},{"apricot","暖杏"}};
            int checked=0;for(String[] p:palettes)for(String style:new String[]{"classic","glass"})for(String theme:new String[]{"light","dark"}){choose("界面风格",style.equals("glass")?"液态玻璃":"经典");choose("浅深色",theme.equals("dark")?"深色":"浅色");click("button[aria-label='配色："+p[1]+"']");synced(p[0],theme,style);pixels(theme.equals("dark"),style.equals("glass"));checked++;}
            choose("界面风格","液态玻璃");choose("浅深色","深色");click("button[aria-label='配色：雾海蓝']");synced("sea","dark","glass");scenario.recreate();scenario.onActivity(a->activity=a);waitFor("document.querySelector('.xm-navigation')");pixels(true,true);assertTrue(js("({ok:document.documentElement.dataset.mobilePalette==='sea'&&document.documentElement.dataset.uiStyle==='glass'&&document.documentElement.classList.contains('dark')})").getBoolean("ok"));assertEquals(before,courseSignature());
            js("(()=>{window.__invalidPalette='WAIT';Capacitor.Plugins.XiqueAppearance.setAppearance({theme:'light',style:'classic',palette:'unknown',mode:'light'}).then(()=>window.__invalidPalette='ACCEPTED').catch(e=>window.__invalidPalette=e.code);return{ok:true}})()");waitFor("window.__invalidPalette!=='WAIT'");assertEquals("INVALID_ARGUMENT",js("({code:window.__invalidPalette})").getString("code"));synced("sea","dark","glass");
            System.out.println("SETTINGS_PALETTES_OK combinations="+checked+" native_css_pixels=true recreate=true invalid_rejected=true private_screenshot_saved=false");
        }
    }
}
