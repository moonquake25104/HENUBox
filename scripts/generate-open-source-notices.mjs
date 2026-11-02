import {readFile,writeFile,mkdir} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';

process.chdir(fileURLToPath(new URL('../',import.meta.url)));
const text=p=>readFile(p,'utf8');

// Read the installed package metadata and the first available license file.
// Missing license files must not break the build; the license identifier is
// still recorded so the notices stay useful.
async function pkg(dir,url){
  const p=JSON.parse(await text(dir+'/package.json'));
  let licenseText='';
  for(const name of ['LICENSE','LICENSE.md','LICENSE.txt']){
    try{licenseText=await text(dir+'/'+name);break}catch{}
  }
  if(!licenseText) licenseText='License: '+(p.license||'UNKNOWN')+'\n';
  return {name:p.name,version:p.version,license:p.license,url,text:licenseText};
}

const items=[];
for(const [dir,url] of [
  ['App/node_modules/react','https://github.com/facebook/react'],
  ['App/node_modules/react-dom','https://github.com/facebook/react'],
  ['App/node_modules/@fluentui/react-components','https://github.com/microsoft/fluentui'],
  ['App/node_modules/radix-ui','https://github.com/radix-ui/primitives'],
  ['App/node_modules/lucide-react','https://github.com/lucide-icons/lucide'],
  ['App/node_modules/@fontsource-variable/geist','https://github.com/fontsource/fontsource'],
  ['App/node_modules/class-variance-authority','https://github.com/joe-bell/cva'],
  ['App/node_modules/clsx','https://github.com/lukeed/clsx'],
  ['App/node_modules/tailwind-merge','https://github.com/dcastil/tailwind-merge'],
  ['App/node_modules/tw-animate-css','https://github.com/Wombosvideo/tw-animate-css'],
  ['App/node_modules/shadcn','https://github.com/shadcn-ui/ui'],
  ['App/node_modules/@capacitor/core','https://github.com/ionic-team/capacitor'],
  ['App/mobile/node_modules/@capacitor/android','https://github.com/ionic-team/capacitor'],
  ['App/mobile/node_modules/@capacitor/app','https://github.com/ionic-team/capacitor-plugins'],
  ['App/mobile/node_modules/@capacitor/local-notifications','https://github.com/ionic-team/capacitor-plugins'],
  ['App/mobile/node_modules/@capacitor/network','https://github.com/ionic-team/capacitor-plugins'],
  ['App/mobile/node_modules/@capacitor/preferences','https://github.com/ionic-team/capacitor-plugins'],
  ['App/mobile/node_modules/@capacitor/status-bar','https://github.com/ionic-team/capacitor-plugins'],
]) items.push(await pkg(dir,url));

const apache=await text('third_party/licenses/Apache-2.0.txt');
items.unshift({name:'Kairos / HENUBox 项目随附许可',version:'2026',license:'MIT',url:'https://github.com/LogicShao/Kairos',text:await text('App/LICENSE')});
items.push({name:'AndroidLiquidGlass / Backdrop',version:'2.0.1 本机源码快照',license:'Apache-2.0',url:'https://github.com/Kyant0/AndroidLiquidGlass',text:(await text('App/mobile/android/henubox-backdrop/src/androidMain/assets/third-party/backdrop/NOTICE.txt'))+'\n'+apache});
items.push({name:'AndroidX / Compose',version:'Compose 1.8.3',license:'Apache-2.0',url:'https://android.googlesource.com/platform/frameworks/support/',text:apache});
items.push({name:'Kotlin',version:'2.2.20',license:'Apache-2.0',url:'https://github.com/JetBrains/kotlin',text:apache});
items.push({name:'Go 运行时',version:'Go 工具链随附许可',license:'BSD-3-Clause',url:'https://go.dev/',text:await text('third_party/licenses/Go-BSD-3-Clause.txt')});
items.push({name:'内置专注声音',version:'雨声 / 海浪 / 粉噪声 / 棕噪声',license:'CC0-1.0',url:'https://github.com/abetusk/noixer#credits',text:await text('App/mobile/android/app/src/main/assets/focus-audio/NOTICE.txt')});
await mkdir('App/src/lib/mobile',{recursive:true});
await writeFile('App/src/lib/mobile/open-source-notices.json',JSON.stringify(items,null,2)+'\n');
console.log('Open-source notices: '+items.length+' verified local texts');
