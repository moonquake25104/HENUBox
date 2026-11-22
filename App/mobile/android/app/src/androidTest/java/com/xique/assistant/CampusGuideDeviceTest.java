package com.xique.assistant;

import android.app.*;
import android.content.*;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.os.*;
import android.view.*;
import androidx.test.core.app.ActivityScenario;
import androidx.test.platform.app.InstrumentationRegistry;
import org.junit.*;
import org.json.*;
import java.io.File;
import java.io.FileOutputStream;
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
    @Test public void largeOfficialMapUsesBoundedRegions()throws Exception{
        guard();Context context=InstrumentationRegistry.getInstrumentation().getTargetContext();GuideResources resources=new GuideResources(context);
        GuideResources.Asset asset=resources.asset("https://www.henu.edu.cn/images/jmxq.jpg",false);assertTrue((long)asset.width*asset.height>50_000_000);BitmapFactory.Options bounds=new BitmapFactory.Options();bounds.inJustDecodeBounds=true;BitmapFactory.decodeFile(asset.preview.getPath(),bounds);assertTrue(Math.max(bounds.outWidth,bounds.outHeight)<=1280);assertTrue(resources.bytes()<=GuideResources.LIMIT);
        Intent intent=new Intent(context,GuideImageActivity.class).putExtra("path",asset.original.getPath()).putExtra("title","金明校区官方地图");
        try(ActivityScenario<GuideImageActivity> scenario=ActivityScenario.launch(intent)){
            AtomicReference<GuideImageActivity.ZoomImageView> view=new AtomicReference<>();scenario.onActivity(a->view.set(find(a.findViewById(android.R.id.content))));long end=System.nanoTime()+TimeUnit.SECONDS.toNanos(15);while(!view.get().ready&&System.nanoTime()<end)Thread.sleep(80);assertTrue(view.get().ready);Thread.sleep(400);
            AtomicReference<float[]> geometry=new AtomicReference<>();scenario.onActivity(a->{GuideImageActivity.ZoomImageView v=view.get();int[] loc=new int[2];v.getLocationOnScreen(loc);geometry.set(new float[]{loc[0]+v.getWidth()/2f,loc[1]+v.getHeight()/2f,v.min});});float[] g=geometry.get();
            for(int i=0;i<2;i++){long now=SystemClock.uptimeMillis();InstrumentationRegistry.getInstrumentation().sendPointerSync(MotionEvent.obtain(now,now,MotionEvent.ACTION_DOWN,g[0],g[1],0));InstrumentationRegistry.getInstrumentation().sendPointerSync(MotionEvent.obtain(now,now+30,MotionEvent.ACTION_UP,g[0],g[1],0));Thread.sleep(70);}Thread.sleep(800);
            scenario.onActivity(a->{GuideImageActivity.ZoomImageView v=view.get();assertTrue(v.scale>g[2]*1.5f);assertTrue(v.preview.getAllocationByteCount()<7*1024*1024);});
            assertNotNull(waitForDetail(scenario, view.get(), null));
            System.out.println("GUIDE_LARGE_MAP_OK source_pixels="+((long)asset.width*asset.height)+" preview_max_edge="+Math.max(bounds.outWidth,bounds.outHeight)+" double_tap=true");
        }
    }
    @Test public void zoomKeepsPreviousDetailUntilNewRegionIsReady() throws Exception {
        guard();
        Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
        File directory = new GuideResources(context).directory;
        assertTrue(directory.isDirectory() || directory.mkdirs());
        checkZoom(context, new File(directory, "zoom-map-fixture.jpg"), 6000, 3900);
        checkZoom(context, new File(directory, "zoom-visits-fixture.png"), 1650, 5100);
    }

    @Test public void pinchAndResetOfficialImageFixtures() throws Exception {
        guard();
        Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
        File directory = new GuideResources(context).directory;
        for (String name : new String[] {"qa-official-map.jpg", "qa-official-visits.jpg"}) {
            File image = new File(directory, name);
            Assume.assumeTrue("Official image fixtures must be supplied by the emulator runner", image.isFile());
            Intent intent = new Intent(context, GuideImageActivity.class).putExtra("path", image.getPath()).putExtra("title", name);
            try (ActivityScenario<GuideImageActivity> scenario = ActivityScenario.launch(intent)) {
                GuideImageActivity.ZoomImageView view = readyViewer(scenario);
                scenario.onActivity(activity -> {
                    float x = view.getWidth() / 2f, y = view.getHeight() / 2f;
                    float original = view.scale;
                    long down = SystemClock.uptimeMillis();
                    touch(view, down, 0, MotionEvent.ACTION_DOWN, x, y, 35, 1);
                    touch(view, down, 10, MotionEvent.ACTION_POINTER_DOWN | (1 << MotionEvent.ACTION_POINTER_INDEX_SHIFT), x, y, 35, 2);
                    for (int i = 1; i <= 18; i++) {
                        touch(view, down, 10 + i * 16, MotionEvent.ACTION_MOVE, x, y, 35 + i * 24, 2);
                        assertNotNull(view.preview);
                    }
                    touch(view, down, 310, MotionEvent.ACTION_POINTER_UP | (1 << MotionEvent.ACTION_POINTER_INDEX_SHIFT), x, y, 467, 2);
                    touch(view, down, 330, MotionEvent.ACTION_UP, x, y, 467, 1);
                    assertTrue(view.scale > original * 1.5f);
                    view.reset();
                    assertEquals(original, view.scale, 0.001);
                });
                System.out.println("GUIDE_OFFICIAL_PINCH_OK " + name);
            }
        }
    }

    private void touch(GuideImageActivity.ZoomImageView view, long down, int elapsed, int action, float x, float y, int halfSpan, int count) {
        MotionEvent.PointerProperties[] properties = new MotionEvent.PointerProperties[count];
        MotionEvent.PointerCoords[] coordinates = new MotionEvent.PointerCoords[count];
        for (int i = 0; i < count; i++) {
            properties[i] = new MotionEvent.PointerProperties();
            properties[i].id = i;
            properties[i].toolType = MotionEvent.TOOL_TYPE_FINGER;
            coordinates[i] = new MotionEvent.PointerCoords();
            coordinates[i].x = x + (i == 0 ? -halfSpan : halfSpan);
            coordinates[i].y = y;
            coordinates[i].pressure = 1;
            coordinates[i].size = 1;
        }
        MotionEvent event = MotionEvent.obtain(down, down + elapsed, action, count, properties, coordinates, 0, 0, 1, 1, 0, 0, InputDevice.SOURCE_TOUCHSCREEN, 0);
        view.dispatchTouchEvent(event);
        event.recycle();
    }

    private GuideImageActivity.ZoomImageView readyViewer(ActivityScenario<GuideImageActivity> scenario) throws InterruptedException {
        AtomicReference<GuideImageActivity.ZoomImageView> reference = new AtomicReference<>();
        scenario.onActivity(activity -> reference.set(find(activity.findViewById(android.R.id.content))));
        GuideImageActivity.ZoomImageView view = reference.get();
        long deadline = SystemClock.uptimeMillis() + 5000;
        while (!view.ready && SystemClock.uptimeMillis() < deadline) Thread.sleep(20);
        assertTrue(view.ready);
        return view;
    }

    private void checkZoom(Context context, File image, int width, int height) throws Exception {
        Bitmap source = Bitmap.createBitmap(width, height, Bitmap.Config.RGB_565);
        Canvas canvas = new Canvas(source);
        canvas.drawColor(0xffe3ebdf);
        Paint ink = new Paint();
        ink.setColor(0xff22382b);
        for (int y = 0; y < height; y += 83) canvas.drawRect(0, y, width, y + 7, ink);
        try (FileOutputStream output = new FileOutputStream(image)) {
            assertTrue(source.compress(image.getName().endsWith(".png") ? Bitmap.CompressFormat.PNG : Bitmap.CompressFormat.JPEG, 88, output));
        } finally {
            source.recycle();
        }

        Intent intent = new Intent(context, GuideImageActivity.class).putExtra("path", image.getPath()).putExtra("title", "Zoom fixture");
        try (ActivityScenario<GuideImageActivity> scenario = ActivityScenario.launch(intent)) {
            GuideImageActivity.ZoomImageView view = readyViewer(scenario);
            scenario.onActivity(activity -> view.zoom(0.95f, view.getWidth() / 2f, view.getHeight() / 2f));
            Bitmap previous = waitForDetail(scenario, view, null);
            assertNotNull(previous);
            scenario.onActivity(activity -> {
                for (float scale : new float[] {0.74f, 0.97f, 1.16f, 1.42f, 1.60f, 2.40f}) {
                    view.zoom(scale, view.getWidth() / 2f, view.getHeight() / 2f);
                    assertSame(previous, view.detail);
                    assertNotNull(view.preview);
                }
            });
            Bitmap next = waitForDetail(scenario, view, previous);
            assertNotNull(next);
            assertTrue(previous.getAllocationByteCount() <= 4L * GuideImageActivity.ZoomImageView.DETAIL_PIXELS);
            assertTrue(next.getAllocationByteCount() <= 4L * GuideImageActivity.ZoomImageView.DETAIL_PIXELS);
            scenario.onActivity(activity -> {
                view.reset();
                assertEquals(view.min, view.scale, 0.001);
                assertNotNull(view.preview);
            });
        } finally {
            image.delete();
        }
        System.out.println("GUIDE_ZOOM_STABLE " + width + "x" + height);
    }

    private Bitmap waitForDetail(ActivityScenario<GuideImageActivity> scenario, GuideImageActivity.ZoomImageView view, Bitmap previous) throws Exception {
        AtomicReference<Bitmap> detail = new AtomicReference<>();
        long deadline = SystemClock.uptimeMillis() + 5000;
        do {
            scenario.onActivity(activity -> detail.set(view.detail));
            if (detail.get() != null && detail.get() != previous) return detail.get();
            Thread.sleep(30);
        } while (SystemClock.uptimeMillis() < deadline);
        return null;
    }

    private GuideImageActivity.ZoomImageView find(View v){if(v instanceof GuideImageActivity.ZoomImageView)return(GuideImageActivity.ZoomImageView)v;if(v instanceof ViewGroup){ViewGroup g=(ViewGroup)v;for(int i=0;i<g.getChildCount();i++){GuideImageActivity.ZoomImageView r=find(g.getChildAt(i));if(r!=null)return r;}}return null;}
    @Test public void clearedGenerationRejectsOldRequests()throws Exception{guard();GuideResources r=new GuideResources(InstrumentationRegistry.getInstrumentation().getTargetContext());long old=r.ticket();r.invalidate();try{r.asset("https://www.henu.edu.cn/images/jmxq.jpg",false,old);fail("outdated asset request accepted");}catch(java.io.IOException expected){assertEquals("RESOURCE_UNAVAILABLE",expected.getMessage());}}
}
