package com.xique.assistant;

/** Generated from src/lib/mobile/palettes.json by scripts/android/sync-presentation-palettes.mjs. */
final class PresentationPalette {
    static boolean valid(String id){return id!=null&&java.util.Arrays.asList("original","pine","sea","lilac","apricot").contains(id);}
    static String[] edges(String id,boolean dark,boolean glass){
        switch(id){
            case "original":return dark?(glass?new String[]{"#1B2C33","#191F29"}:new String[]{"#18191C","#18191C"}):(glass?new String[]{"#E9F2F3","#EEF1F8"}:new String[]{"#FFFFFF","#FFFFFF"});
            case "pine":return dark?(glass?new String[]{"#1B3025","#18271F"}:new String[]{"#17211C","#17211C"}):(glass?new String[]{"#E7F0E7","#EDF1E5"}:new String[]{"#F4F7F3","#F4F7F3"});
            case "sea":return dark?(glass?new String[]{"#1A2E42","#192437"}:new String[]{"#17202B","#17202B"}):(glass?new String[]{"#E6EEF7","#E8EEF8"}:new String[]{"#F3F6FA","#F3F6FA"});
            case "lilac":return dark?(glass?new String[]{"#31213F","#251F36"}:new String[]{"#211A29","#211A29"}):(glass?new String[]{"#F0E7F5","#ECE8F7"}:new String[]{"#F7F4FA","#F7F4FA"});
            case "apricot":return dark?(glass?new String[]{"#382B1E","#2C231D"}:new String[]{"#271F18","#271F18"}):(glass?new String[]{"#F7ECDC","#F3EAE2"}:new String[]{"#FBF6EF","#FBF6EF"});
            default:throw new IllegalArgumentException("INVALID_ARGUMENT");
        }
    }
}
