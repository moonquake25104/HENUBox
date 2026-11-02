package com.xique.assistant;

import android.app.*;
import android.content.*;
import android.graphics.BitmapFactory;
import android.os.*;
import android.view.*;
import androidx.test.core.app.ActivityScenario;
import androidx.test.platform.app.InstrumentationRegistry;
import org.junit.*;
import org.json.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicReference;
import static org.junit.Assert.*;

/** Live public resources only. Requires an explicitly disposable emulator. */
public class CampusGuideDeviceTest {
    private MainActivity activity;
    private void guard(){Assume.assumeTrue(Build.HARDWARE.contains("ranchu")||Build.HARDWARE.contains("goldfish"));assertEquals("true",InstrumentationRegistry.getArguments().getString("allow_disposable_workspace"));}
    private JSONObject js(String expression)throws Exception{CountDownLatch done=new CountDownLatch(1);AtomicReference<String> raw=new AtomicReference<>();InstrumentationRegistry.getInstrumentation().runOnMainSync(()->activity.getBridge().getWebView().evaluateJavascript("JSON.stringify(("+expression+"))",v->{raw.set(v);done.countDown();}));assertTrue(done.await(10,TimeUnit.SECONDS));Object value=new JSONTokener(raw.get()).nextValue();return new JSONObject((String)value);}
    private void waitFor(String expression,int seconds)throws Exception{long end=System.nanoTime()+TimeUnit.SECONDS.toNanos(seconds);while(System.nanoTime()<end){try{if(js("({ready:!!("+expression+")})").getBoolean("ready"))return;}catch(Exception|AssertionError ignored){}Thread.sleep(100);}fail("Guide UI condition timed out");}
    private JSONObject call(String method,String args)throws Exception{js("(()=>{delete window.__guideDeviceResult;Capacitor.nativePromise('XiqueGuide','"+method+"',"+args+").then(v=>window.__guideDeviceResult={value:v}).catch(e=>window.__guideDeviceResult={error:e.code||'ERROR'});return {ok:true}})()");waitFor("window.__guideDeviceResult",45);return js("window.__guideDeviceResult");}
    private void markCourses(String name)throws Exception{js("(()=>{window.__"+name+"Ready=false;Capacitor.Plugins.XiqueLocal.bootstrap().then(async b=>{const bytes=new TextEncoder().encode(JSON.stringify({revision:b.revision,courses:b.courses})),d=await crypto.subtle.digest('SHA-256',bytes);window.__"+name+"=Array.from(new Uint8Array(d)).map(x=>x.toString(16).padStart(2,'0')).join('');window.__"+name+"Ready=true});return {ok:true}})()");waitFor("window.__"+name+"Ready",20);}
    @Test public void liveBridgeAndPublicPages()throws Exception{
        guard();try(ActivityScenario<MainActivity> scenario=ActivityScenario.launch(MainActivity.class)){scenario.onActivity(a->activity=a);waitFor("document.querySelector('.xm-navigation')",25);markCourses("guideBefore");
            assertEquals("INVALID_ARGUMENT",call("getPage","{kind:'login'}").getString("error"));
            assertEquals("INVALID_ARGUMENT",call("getAsset","{url:'https://evil.test/image.jpg',refresh:false}").getString("error"));
            assertEquals("INVALID_ARGUMENT",call("dial","{number:'*123#'}").getString("error"));
            for(String kind:new String[]{"maps","phones","emblem","anthem"}){JSONObject result=call("getPage","{kind:'"+kind+"'}");assertFalse(result.toString(),result.has("error"));assertTrue(result.getJSONObject("value").getString("html").contains("河南大学"));}
            assertFalse(call("cacheInfo","{}").has("error"));markCourses("guideAfter");assertTrue(js("({same:window.__guideBefore===window.__guideAfter})").getBoolean("same"));
            System.out.println("GUIDE_LIVE_BRIDGE_OK public_pages=4 argument_rejections=3 course_fingerprint_unchanged=true");
        }
    }
    @Test public void largeOfficialMapUsesBoundedTiles()throws Exception{
        guard();Context context=InstrumentationRegistry.getInstrumentation().getTargetContext();GuideResources resources=new GuideResources(context);
        GuideResources.Asset asset=resources.asset("https://www.henu.edu.cn/images/jmxq.jpg",false);assertTrue((long)asset.width*asset.height>50_000_000);BitmapFactory.Options bounds=new BitmapFactory.Options();bounds.inJustDecodeBounds=true;BitmapFactory.decodeFile(asset.preview.getPath(),bounds);assertTrue(Math.max(bounds.outWidth,bounds.outHeight)<=1280);assertTrue(resources.bytes()<=GuideResources.LIMIT);
        Intent intent=new Intent(context,GuideImageActivity.class).putExtra("path",asset.original.getPath()).putExtra("title","金明校区官方地图");
        try(ActivityScenario<GuideImageActivity> scenario=ActivityScenario.launch(intent)){
            AtomicReference<GuideImageActivity.TileView> tile=new AtomicReference<>();scenario.onActivity(a->tile.set(find(a.findViewById(android.R.id.content))));long end=System.nanoTime()+TimeUnit.SECONDS.toNanos(15);while(!tile.get().ready&&System.nanoTime()<end)Thread.sleep(80);assertTrue(tile.get().ready);Thread.sleep(400);
            AtomicReference<float[]> geometry=new AtomicReference<>();scenario.onActivity(a->{GuideImageActivity.TileView v=tile.get();int[] loc=new int[2];v.getLocationOnScreen(loc);geometry.set(new float[]{loc[0]+v.getWidth()/2f,loc[1]+v.getHeight()/2f,v.min});});float[] g=geometry.get();
            for(int i=0;i<2;i++){long now=SystemClock.uptimeMillis();InstrumentationRegistry.getInstrumentation().sendPointerSync(MotionEvent.obtain(now,now,MotionEvent.ACTION_DOWN,g[0],g[1],0));InstrumentationRegistry.getInstrumentation().sendPointerSync(MotionEvent.obtain(now,now+30,MotionEvent.ACTION_UP,g[0],g[1],0));Thread.sleep(70);}Thread.sleep(800);
            scenario.onActivity(a->{GuideImageActivity.TileView v=tile.get();assertTrue(v.scale>g[2]*1.5f);assertTrue(v.cache.size()<=8*1024*1024);assertTrue(v.preview.getAllocationByteCount()<7*1024*1024);});
            System.out.println("GUIDE_LARGE_MAP_OK source_pixels="+((long)asset.width*asset.height)+" preview_max_edge="+Math.max(bounds.outWidth,bounds.outHeight)+" tile_cache_limit_bytes=8388608 double_tap=true");
        }
    }
    private GuideImageActivity.TileView find(View v){if(v instanceof GuideImageActivity.TileView)return(GuideImageActivity.TileView)v;if(v instanceof ViewGroup){ViewGroup g=(ViewGroup)v;for(int i=0;i<g.getChildCount();i++){GuideImageActivity.TileView r=find(g.getChildAt(i));if(r!=null)return r;}}return null;}
    @Test public void clearedGenerationRejectsOldRequests()throws Exception{guard();GuideResources r=new GuideResources(InstrumentationRegistry.getInstrumentation().getTargetContext());long old=r.ticket();r.invalidate();try{r.asset("https://www.henu.edu.cn/images/jmxq.jpg",false,old);fail("outdated asset request accepted");}catch(java.io.IOException expected){assertEquals("RESOURCE_UNAVAILABLE",expected.getMessage());}}
}
