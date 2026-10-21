#!/usr/bin/env node

// Generates locally bundled React icons from Hugeicons free Stroke Rounded SVGs.
// Usage: pnpm icon arrow-right-02-stroke-rounded [other-icon-name-or-URL...]

import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../src/shared/lib/icons");
const cdn = "https://cdn.hugeicons.com/icons/";

const pascal = (value) => value.replace(/(^|-)([a-z0-9])/g, (_, __, letter) => letter.toUpperCase());

const getIcon = (input) => {
  const url = /^https?:\/\//.test(input) ? new URL(input) : new URL(`${input.replace(/\.svg$/, "")}.svg?v=2.0`, cdn);
  if (url.origin !== "https://cdn.hugeicons.com" || !url.pathname.startsWith("/icons/")) {
    throw new Error("Разрешены только SVG с официального CDN Hugeicons");
  }

  const filename = path.basename(url.pathname);
  if (!/^[a-z][a-z0-9-]+-stroke-rounded\.svg$/.test(filename)) {
    throw new Error(`Для репозитория разрешены только бесплатные MIT-иконки stroke-rounded: ${filename}`);
  }
  return { name: filename.slice(0, -4), url: url.toString() };
};

const createComponent = (svg, name) => {
  const match = svg.match(/^\s*<svg\b([^>]*)>([\s\S]*?)<\/svg>\s*$/i);
  if (!match || /<(?:script|foreignObject|image|use)\b/i.test(match[2])) {
    throw new Error(`Некорректный или небезопасный SVG: ${name}`);
  }

  const viewBox = match[1].match(/\bviewBox="([\d\s.\-]+)"/)?.[1] ?? "0 0 24 24";
  const inner = match[2].trim()
    .replace(/\s+stroke-width="[^"]*"/gi, "")
    .replace(/\b(stroke|fill)="#[0-9a-fA-F]{3,8}"/g, '$1="currentColor"')
    .replace(/\b(stroke-linecap|stroke-linejoin|fill-rule|clip-rule|stroke-dasharray|stroke-dashoffset)="/g,
      (_, attribute) => `${attribute.replace(/-([a-z])/g, (__, letter) => letter.toUpperCase())}="`);

  const component = `${pascal(name)}Icon`;
  return {
    component,
    code: `import type { SVGProps } from "react";

type Props = SVGProps<SVGSVGElement> & { size?: number | string };

// Source: Hugeicons Free Stroke Rounded (MIT), https://hugeicons.com/icons
export const ${component} = ({ size = 24, width, height, color = "currentColor", strokeWidth = 1.5, ...props }: Props) => (
  <svg
    width={width ?? size}
    height={height ?? size}
    viewBox="${viewBox}"
    fill="none"
    color={color}
    strokeWidth={strokeWidth}
    xmlns="http://www.w3.org/2000/svg"
    {...props}
  >
    ${inner.replace(/\n/g, "\n    ")}
  </svg>
);
`,
  };
};

const main = async () => {
  if (process.argv.length < 3) {
    throw new Error("Использование: pnpm icon arrow-up-right-01-stroke-rounded [другие имена или URL]");
  }

  await mkdir(root, { recursive: true });
  const results = [];
  for (const input of process.argv.slice(2)) {
    const { name, url } = getIcon(input);
    const response = await fetch(url);
    if (!response.ok) throw new Error(`Hugeicons CDN: ${response.status} — ${name}`);
    const svg = await response.text();
    const { component, code } = createComponent(svg, name);
    await writeFile(path.join(root, `${name}.tsx`), code, "utf8");
    results.push({ name, component });
    console.log(`Скачано: ${name}.svg → ${component}`);
  }

  const indexPath = path.join(root, "index.ts");
  const current = await readFile(indexPath, "utf8").catch(() => "");
  const exports = new Set(current.split(/\r?\n/).filter(Boolean));
  for (const { name, component } of results) {
    exports.add(`export { ${component} } from "./${name}";`);
  }
  await writeFile(indexPath, `${[...exports].sort().join("\n")}\n`, "utf8");
};

main().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
