#!/usr/bin/env node

const esbuild = require('esbuild')
const alias = require('esbuild-plugin-alias')
const nodeGlobals = require('@esbuild-plugins/node-globals-polyfill').default

esbuild
  .build({
    entryPoints: ['globals.js'],
    outfile: 'globals.bundle.js',
    bundle: true,
    plugins: [
      alias({
        stream: require.resolve('readable-stream')
      }),
      nodeGlobals({buffer: true})
    ],
    define: {
      window: 'self',
      global: 'self'
    }
})
  .then(() => console.log('build success.'))
