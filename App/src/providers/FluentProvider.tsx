import { FluentProvider as FluentUIProvider, webLightTheme, webDarkTheme, type Theme } from '@fluentui/react-components';
import { type ReactNode, useEffect, useState } from 'react';

interface FluentProviderProps {
  children: ReactNode;
}

export function FluentProvider({ children }: FluentProviderProps) {
  const [theme, setTheme] = useState<Theme>(() => {
    const stored = localStorage.getItem("kairos-theme");
    if (stored === "dark") return webDarkTheme;
    if (stored === "light") return webLightTheme;
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? webDarkTheme : webLightTheme;
  });

  useEffect(() => {
    const handleStorageChange = () => {
      const stored = localStorage.getItem("kairos-theme");
      setTheme(stored === "dark" ? webDarkTheme : webLightTheme);
    };

    window.addEventListener('storage', handleStorageChange);
    // 自定义事件用于同步主题变化
    window.addEventListener('theme-change', handleStorageChange);

    return () => {
      window.removeEventListener('storage', handleStorageChange);
      window.removeEventListener('theme-change', handleStorageChange);
    };
  }, []);

  return (
    <FluentUIProvider theme={theme}>
      {children}
    </FluentUIProvider>
  );
}
