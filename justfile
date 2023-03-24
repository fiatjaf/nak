cloudflare:
    rm -fr cf
    mkdir -p cf/target/esbuild
    cp index.html cf/
    cp target/esbuild/bundle.js cf/target/esbuild
    wrangler pages publish cf --project-name nostr-army-knife --branch master
    rm -fr cf
