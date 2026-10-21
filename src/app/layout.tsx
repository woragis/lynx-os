import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Lynx OS · Central de operações",
  description: "Provisionamento central dos sistemas Lynx",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="pt-BR"
    >
      <body>{children}</body>
    </html>
  );
}
