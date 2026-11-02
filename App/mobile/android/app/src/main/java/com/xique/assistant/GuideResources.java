package com.xique.assistant;

import android.content.Context;
import android.graphics.*;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.*;
import java.util.concurrent.atomic.AtomicLong;

/** Public, cookieless guide resources. Never uses school credentials or local databases. */
final class GuideResources {
    static final String HOST="www.henu.edu.cn", ANTHEM="https://www.henu.edu.cn/mp3/xiaoge.mp3";
    static final Map<String,String> PAGES=pairs("maps","https://www.henu.edu.cn/zjhd/xqsyt.htm","calendar","https://www.henu.edu.cn/xyfw/xl.htm","phones","https://www.henu.edu.cn/xyfw/xndh.htm","shuttle","https://www.henu.edu.cn/xyfw/xccx.htm","visits","https://www.henu.edu.cn/xyfw/yyjx.htm","emblem","https://www.henu.edu.cn/zjhd/xhxxxf.htm","anthem","https://www.henu.edu.cn/zjhd/hndxxg.htm","vr","https://www.720yun.com/vr/e4b2f9rfu4a");
    static final Map<String,String> TITLES=pairs("maps","校园地图","calendar","校历","phones","校内电话","shuttle","校车查询","visits","预约进校","emblem","校徽校训校风","anthem","河南大学校歌","vr","校史馆 VR 全景");
    private static Map<String,String> pairs(String... values){Map<String,String> result=new LinkedHashMap<>();for(int i=0;i<values.length;i+=2)result.put(values[i],values[i+1]);return Collections.unmodifiableMap(result);}
    static final long LIMIT=64L*1024*1024, FILE_LIMIT=20L*1024*1024, ASSET_TTL=7L*86400000;
    final File directory;
    private final AtomicLong epoch=new AtomicLong();
    long ticket(){return epoch.get();}
    void invalidate(){epoch.incrementAndGet();}
    GuideResources(Context c){directory=new File(c.getCacheDir(),"campus-guide-v1");}
    static boolean assetUrl(String s){try{URI u=new URI(s);String p=u.getRawPath();return "https".equals(u.getScheme())&&HOST.equals(u.getHost())&&(u.getPort()==-1||u.getPort()==443)&&u.getRawUserInfo()==null&&u.getRawQuery()==null&&u.getRawFragment()==null&&u.normalize().equals(u)&&p!=null&&!p.contains("%")&&(s.equals(ANTHEM)||(p.startsWith("/images/")||p.startsWith("/__local/"))&&p.matches("[A-Za-z0-9_./-]+\\.(?:png|jpg|jpeg|gif)"));}catch(Exception e){return false;}}
    private HttpURLConnection open(String value,boolean page)throws IOException{
        String next=value;
        for(int i=0;i<4;i++){
            if(page?!PAGES.containsValue(next)||next.equals(PAGES.get("vr")):!assetUrl(next))throw new IOException("RESOURCE_UNAVAILABLE");
            HttpURLConnection c=(HttpURLConnection)new URL(next).openConnection();c.setConnectTimeout(12000);c.setReadTimeout(18000);c.setInstanceFollowRedirects(false);c.setRequestProperty("User-Agent","Mozilla/5.0 HENUBox/0.1 CampusGuide");c.setRequestProperty("Accept-Encoding","identity");c.setRequestProperty("Cookie","");
            int status=c.getResponseCode();
            if(status>=300&&status<400){String location=c.getHeaderField("Location");c.disconnect();if(location==null)throw new IOException("RESOURCE_UNAVAILABLE");next=new URL(new URL(next),location).toString();continue;}
            if(status!=200){c.disconnect();throw new IOException("RESOURCE_UNAVAILABLE");}return c;
        }throw new IOException("RESOURCE_UNAVAILABLE");
    }
    static void copyBounded(InputStream in,OutputStream out,long limit)throws IOException{byte[] b=new byte[16384];long total=0;int n;while((n=in.read(b))!=-1){total+=n;if(total>limit)throw new IOException("RESOURCE_TOO_LARGE");out.write(b,0,n);}}
    String page(String kind)throws IOException{
        String url=PAGES.get(kind);if(url==null||kind.equals("vr"))throw new IllegalArgumentException("INVALID_ARGUMENT");
        HttpURLConnection c=open(url,true);try(InputStream in=c.getInputStream();ByteArrayOutputStream out=new ByteArrayOutputStream()){copyBounded(in,out,1024*1024);String html=out.toString(StandardCharsets.UTF_8.name());if(!html.contains("nyrcon")||!html.contains("河南大学"))throw new IOException("RESOURCE_UNAVAILABLE");return html;}finally{c.disconnect();}
    }
    static String hash(String value){try{byte[] d=MessageDigest.getInstance("SHA-256").digest(value.getBytes(StandardCharsets.UTF_8));StringBuilder b=new StringBuilder();for(byte v:d)b.append(String.format(Locale.ROOT,"%02x",v&255));return b.toString();}catch(Exception e){throw new IllegalStateException(e);}}
    static final class Asset{final File original,preview;final int width,height;final boolean stale;Asset(File a,File p,int w,int h,boolean s){original=a;preview=p;width=w;height=h;stale=s;}}
    Asset asset(String url,boolean refresh)throws IOException{return asset(url,refresh,ticket());}
    synchronized Asset asset(String url,boolean refresh,long ticket)throws IOException{
        if(ticket!=epoch.get())throw new IOException("RESOURCE_UNAVAILABLE");
        if(!assetUrl(url))throw new IllegalArgumentException("INVALID_ARGUMENT");if(!directory.isDirectory()&&!directory.mkdirs())throw new IOException("RESOURCE_UNAVAILABLE");
        boolean audio=url.equals(ANTHEM);String key=hash(url);File f=new File(directory,key+(audio?".mp3":".image")),p=new File(directory,key+".preview.png");boolean stale=false;
        if(refresh||!f.isFile()||System.currentTimeMillis()-f.lastModified()>ASSET_TTL){
            File part=new File(directory,key+".part");try{HttpURLConnection c=open(url,false);try{long n=c.getContentLengthLong();if(n>FILE_LIMIT)throw new IOException("RESOURCE_TOO_LARGE");try(InputStream in=c.getInputStream();OutputStream out=new FileOutputStream(part)){copyBounded(in,out,FILE_LIMIT);}}finally{c.disconnect();}
                validate(part,audio);if(ticket!=epoch.get())throw new IOException("RESOURCE_UNAVAILABLE");if(!part.renameTo(f))throw new IOException("RESOURCE_UNAVAILABLE");p.delete();
            }catch(IOException e){part.delete();if(ticket!=epoch.get()||!f.isFile())throw e;stale=true;}
        }
        int w=0,h=0;if(!audio){BitmapFactory.Options bounds=validate(f,false);w=bounds.outWidth;h=bounds.outHeight;if(!p.isFile()){
                BitmapFactory.Options options=new BitmapFactory.Options();int sample=1;while(Math.max(w,h)/sample>1280)sample*=2;options.inSampleSize=sample;Bitmap bitmap=BitmapFactory.decodeFile(f.getPath(),options);if(bitmap==null)throw new IOException("RESOURCE_UNAVAILABLE");File temp=new File(directory,key+".thumb.part");try(OutputStream out=new FileOutputStream(temp)){if(!bitmap.compress(Bitmap.CompressFormat.PNG,100,out))throw new IOException("RESOURCE_UNAVAILABLE");}finally{bitmap.recycle();}if(!temp.renameTo(p)){temp.delete();throw new IOException("RESOURCE_UNAVAILABLE");}
            }}
        prune(f,p);return new Asset(f,audio?f:p,w,h,stale);
    }
    Asset cachedImage(String url)throws IOException{
        if(!assetUrl(url)||url.equals(ANTHEM))throw new IllegalArgumentException("INVALID_ARGUMENT");
        String key=hash(url);File f=new File(directory,key+".image"),p=new File(directory,key+".preview.png");
        if(!f.isFile())throw new IOException("RESOURCE_UNAVAILABLE");BitmapFactory.Options bounds=validate(f,false);
        return new Asset(f,p,bounds.outWidth,bounds.outHeight,System.currentTimeMillis()-f.lastModified()>ASSET_TTL);
    }
    private BitmapFactory.Options validate(File f,boolean audio)throws IOException{
        if(f.length()==0||f.length()>FILE_LIMIT)throw new IOException("RESOURCE_TOO_LARGE");
        if(audio){try(InputStream in=new FileInputStream(f)){int a=in.read(),b=in.read(),c=in.read();if(!(a=='I'&&b=='D'&&c=='3')&&!(a==255&&(b&224)==224))throw new IOException("RESOURCE_UNAVAILABLE");}return null;}
        BitmapFactory.Options o=new BitmapFactory.Options();o.inJustDecodeBounds=true;BitmapFactory.decodeFile(f.getPath(),o);if(o.outWidth<=0||o.outHeight<=0||(long)o.outWidth*o.outHeight>100_000_000L)throw new IOException("RESOURCE_TOO_LARGE");return o;
    }
    private void prune(File keep,File preview){File[] files=directory.listFiles();if(files==null)return;Arrays.sort(files,Comparator.comparingLong(File::lastModified));long size=0;for(File f:files)size+=f.length();for(File f:files){if(size<=LIMIT)break;if(!f.equals(keep)&&!f.equals(preview)&&f.isFile()){long n=f.length();if(f.delete())size-=n;}}}
    synchronized long bytes(){long n=0;File[] files=directory.listFiles();if(files!=null)for(File f:files)if(f.isFile())n+=f.length();return n;}
    synchronized void clear()throws IOException{File[] files=directory.listFiles();if(files!=null)for(File f:files)if(f.isFile()&&!f.delete())throw new IOException("RESOURCE_UNAVAILABLE");}
}
