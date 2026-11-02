package com.xique.assistant;

import android.Manifest;
import android.content.*;
import android.net.Uri;
import android.os.Build;
import android.os.Looper;
import android.provider.MediaStore;
import com.getcapacitor.*;
import com.getcapacitor.annotation.*;
import java.io.*;
import java.util.concurrent.*;

@CapacitorPlugin(name="XiqueGuide",permissions={@Permission(alias="publicImages",strings={Manifest.permission.WRITE_EXTERNAL_STORAGE})})
public final class XiqueGuidePlugin extends Plugin {
    private final ExecutorService io=Executors.newFixedThreadPool(2);
    private final ExecutorService localIo=Executors.newSingleThreadExecutor();
    private GuideResources resources;
    @Override public void load(){resources=new GuideResources(getContext());}
    private String text(PluginCall c,String key){Object value=c.getData().opt(key);return value instanceof String?(String)value:null;}
    private boolean trusted(){try{Callable<Boolean> read=()->SessionPolicy.trustedPage(getBridge().getWebView().getUrl())&&getBridge().getServerUrl()==null;if(Looper.myLooper()==Looper.getMainLooper())return read.call();FutureTask<Boolean> f=new FutureTask<>(read);getActivity().runOnUiThread(f);return f.get(2,TimeUnit.SECONDS);}catch(Exception e){return false;}}
    private boolean check(PluginCall c,String...fields){if(!trusted()){c.reject("Untrusted page","UNTRUSTED_ORIGIN");return false;}try{BridgeArgs.exact(c,fields);return true;}catch(Exception e){c.reject("Invalid guide arguments","INVALID_ARGUMENT");return false;}}
    private void fail(PluginCall c,Exception e){String code="INVALID_ARGUMENT".equals(e.getMessage())?"INVALID_ARGUMENT":"RESOURCE_TOO_LARGE".equals(e.getMessage())?"RESOURCE_TOO_LARGE":"RESOURCE_UNAVAILABLE";c.reject("Campus guide resource unavailable",code);}
    @PluginMethod public void getPage(PluginCall c){if(!check(c,"kind"))return;String k=text(c,"kind");if(!GuideResources.PAGES.containsKey(k==null?"":k)||"vr".equals(k)){c.reject("Invalid kind","INVALID_ARGUMENT");return;}io.execute(()->{try{c.resolve(new JSObject().put("html",resources.page(k)));}catch(Exception e){fail(c,e);}});}
    @PluginMethod public void getAsset(PluginCall c){if(!check(c,"url","refresh"))return;String u=text(c,"url");Object flag=c.getData().opt("refresh");if(!GuideResources.assetUrl(u)||flag!=null&&!(flag instanceof Boolean)){c.reject("Invalid asset","INVALID_ARGUMENT");return;}long ticket=resources.ticket();io.execute(()->{try{GuideResources.Asset a=resources.asset(u,Boolean.TRUE.equals(flag),ticket);c.resolve(new JSObject().put("uri",Uri.fromFile(a.preview).toString()).put("width",a.width).put("height",a.height).put("bytes",a.original.length()).put("stale",a.stale));}catch(Exception e){fail(c,e);}});}
    @PluginMethod public void viewImage(PluginCall c){if(!check(c,"url","title"))return;String u=text(c,"url"),t=text(c,"title");if(!GuideResources.assetUrl(u)||GuideResources.ANTHEM.equals(u)||t==null||t.isEmpty()||t.length()>80){c.reject("Invalid image","INVALID_ARGUMENT");return;}localIo.execute(()->{try{GuideResources.Asset a=resources.cachedImage(u);getActivity().runOnUiThread(()->{try{getActivity().startActivity(new Intent(getActivity(),GuideImageActivity.class).putExtra("path",a.original.getPath()).putExtra("title",t));c.resolve();}catch(Exception e){c.reject("Image unavailable","BROWSER_UNAVAILABLE");}});}catch(Exception e){fail(c,e);}});}
    @PluginMethod public void openPage(PluginCall c){if(!check(c,"kind"))return;String k=text(c,"kind");if(!GuideResources.PAGES.containsKey(k==null?"":k)){c.reject("Invalid kind","INVALID_ARGUMENT");return;}getActivity().runOnUiThread(()->{try{getActivity().startActivity(new Intent(getActivity(),GuideBrowserActivity.class).putExtra("kind",k));c.resolve();}catch(Exception e){c.reject("Guide browser unavailable","BROWSER_UNAVAILABLE");}});}
    @PluginMethod public void dial(PluginCall c){if(!check(c,"number"))return;String n=text(c,"number");if(n==null||!n.matches("(?:0[0-9]{9,11}|1[0-9]{10}|400[0-9]{7})")){c.reject("Invalid phone number","INVALID_ARGUMENT");return;}getActivity().runOnUiThread(()->{try{getActivity().startActivity(new Intent(Intent.ACTION_DIAL,Uri.parse("tel:"+n)));c.resolve();}catch(Exception e){c.reject("Dialer unavailable","BROWSER_UNAVAILABLE");}});}
    @PluginMethod public void cacheInfo(PluginCall c){if(check(c))io.execute(()->c.resolve(new JSObject().put("bytes",resources.bytes())));}
    @PluginMethod public void clearCache(PluginCall c){if(!check(c))return;resources.invalidate();io.execute(()->{try{resources.clear();c.resolve();}catch(Exception e){fail(c,e);}});}
    @PluginMethod public void saveVisitGuide(PluginCall c){if(!check(c,"url"))return;if(!GuideResources.assetUrl(text(c,"url"))||GuideResources.ANTHEM.equals(text(c,"url"))){c.reject("Invalid image","INVALID_ARGUMENT");return;}if(Build.VERSION.SDK_INT<29&&getPermissionState("publicImages")!=PermissionState.GRANTED){requestPermissionForAlias("publicImages",c,"imagePermission");return;}save(c);}
    @PermissionCallback private void imagePermission(PluginCall c){if(getPermissionState("publicImages")==PermissionState.GRANTED)save(c);else c.reject("Image save permission denied","PERMISSION_DENIED");}
    private void save(PluginCall c){io.execute(()->{
        Uri uri=null;
        try {
            GuideResources.Asset a=resources.asset(text(c,"url"),false);
            android.graphics.BitmapFactory.Options bounds=new android.graphics.BitmapFactory.Options();bounds.inJustDecodeBounds=true;android.graphics.BitmapFactory.decodeFile(a.original.getPath(),bounds);
            String mime=bounds.outMimeType;if(mime==null||!mime.startsWith("image/"))throw new IOException();
            String extension=mime.equals("image/png")?".png":mime.equals("image/gif")?".gif":".jpg";
            ContentValues values=new ContentValues();values.put(MediaStore.Images.Media.DISPLAY_NAME,"HENUBox-访客流程-"+System.currentTimeMillis()+extension);values.put(MediaStore.Images.Media.MIME_TYPE,mime);
            if(Build.VERSION.SDK_INT>=29){values.put(MediaStore.Images.Media.RELATIVE_PATH,"Pictures/HENUBox");values.put(MediaStore.Images.Media.IS_PENDING,1);}
            uri=getContext().getContentResolver().insert(MediaStore.Images.Media.EXTERNAL_CONTENT_URI,values);if(uri==null)throw new IOException();
            try(InputStream in=new FileInputStream(a.original);OutputStream out=getContext().getContentResolver().openOutputStream(uri)){if(out==null)throw new IOException();GuideResources.copyBounded(in,out,GuideResources.FILE_LIMIT);}
            if(Build.VERSION.SDK_INT>=29){values.clear();values.put(MediaStore.Images.Media.IS_PENDING,0);getContext().getContentResolver().update(uri,values,null,null);}c.resolve();
        } catch(Exception e){if(uri!=null)getContext().getContentResolver().delete(uri,null,null);fail(c,e);}
    });}
    @Override protected void handleOnDestroy(){io.shutdownNow();localIo.shutdownNow();}
}
