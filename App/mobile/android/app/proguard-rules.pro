# Add project specific ProGuard rules here.
# You can control the set of applied configuration files using the
# proguardFiles setting in build.gradle.
#
# For more details, see
#   http://developer.android.com/guide/developing/tools/proguard.html

# If your project uses WebView with JS, uncomment the following
# and specify the fully qualified class name to the JavaScript interface
# class:
#-keepclassmembers class fqcn.of.javascript.interface.for.webview {
#   public *;
#}

# Uncomment this to preserve the line number information for
# debugging stack traces.
#-keepattributes SourceFile,LineNumberTable

# If you keep the line number information, uncomment this to
# hide the original source file name.
#-renamesourcefileattribute SourceFile
# Keep the existing application and JNI/bridge contracts while shrinking unused
# Compose library code. The original signing key and debug build type are retained.
-keep class com.xique.assistant.** { *; }
-keep class com.henubox.glass.** { *; }
-keep class com.kyant.backdrop.** { *; }
-keep class go.** { *; }
-keep class henuparse.** { *; }
-keepclassmembers class * { @android.webkit.JavascriptInterface <methods>; }
