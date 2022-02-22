#!/usr/bin/env node

require('esbuild').buildSync({
  entryPoints: ['globals.js'],
  outfile: 'globals.bundle.js',
  bundle: true
})
