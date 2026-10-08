import { build } from 'esbuild';
import { readFile, writeFile } from 'node:fs/promises';
const bundle = await build({ entryPoints: ['ui/card.js'], bundle: true, write: false, format: 'iife', minify: true, target: 'es2022' });
const template = await readFile('ui/template.html', 'utf8');
await writeFile('ui/card.html', template.replace('/* BUNDLE */', () => bundle.outputFiles[0].text.replace(/<\/script/gi, '<\\/script')));
