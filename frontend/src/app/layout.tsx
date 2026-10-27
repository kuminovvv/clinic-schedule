import type {Metadata} from "next";
import localFont from "next/font/local";
import {AppHeader} from "@/widgets/app-header/ui/app-header";
import {Toaster} from "@/shared/ui/shadcn/toast";
import "react-day-picker/style.css";
import "./globals.css";

// Official Geist font files from the pinned `geist` package, bundled locally.
// Explicit italic sources prevent the browser from synthesizing italic glyphs.
const geistSans = localFont({
    src: [
        {path: "../../node_modules/geist/dist/fonts/geist-sans/Geist-Variable.woff2", style: "normal", weight: "100 900"},
        {path: "../../node_modules/geist/dist/fonts/geist-sans/Geist-Italic[wght].woff2", style: "italic", weight: "100 900"},
    ],
    variable: "--font-geist-sans",
    display: "swap",
});

const geistMono = localFont({
    src: [
        {path: "../../node_modules/geist/dist/fonts/geist-mono/GeistMono-Variable.woff2", style: "normal", weight: "100 900"},
        {path: "../../node_modules/geist/dist/fonts/geist-mono/GeistMono-Italic[wght].woff2", style: "italic", weight: "100 900"},
    ],
    variable: "--font-geist-mono",
    display: "swap",
    preload: false,
});

export const metadata: Metadata = {
    title: "Clinic Schedule",
    description: "Clinic schedule management system.",
};

export default function RootLayout({children}: LayoutProps<"/">) {
    return (
        <html
            lang="ru"
            className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
        >
        <body className="min-h-full flex flex-col pt-16">
        <AppHeader/>
        {children}
        <Toaster/>
        </body>
        </html>
    );
}
