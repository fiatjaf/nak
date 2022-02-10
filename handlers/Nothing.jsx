import React from 'react'

export default function Nothing() {
  return (
    <>
      you can paste
      <ul>
        <li>an unsigned event to be hashed and signed</li>
        <li>a signed event to have its signature checked</li>
        <li>a nostr relay URL to be inspected</li>
        <li>a nostr event id we'll try to fetch</li>
        <li>a nip05 identifier to be checked</li>
        <li>
          contribute a new function:{' '}
          <a
            target="_blank"
            style={{color: 'inherit'}}
            href="https://github.com/fiatjaf/nostr-army-knife"
          >
            _______
          </a>
        </li>
      </ul>
    </>
  )
}

Nothing.match = () => true
