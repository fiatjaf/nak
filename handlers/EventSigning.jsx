import React, {useState} from 'react'
import useComputedState from 'use-computed-state'
import {
  getPublicKey,
  getEventHash,
  serializeEvent,
  verifySignature,
  signEvent
} from 'nostr-tools'

import Item from '../components/item'

export default function EventSigning({value}) {
  let evt = JSON.parse(value)
  let [privateKey, setPrivateKey] = useState(
    'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
  )
  let privateKeyIsValid = useComputedState(
    () => privateKey.match(/[a-f0-9]{64}/),
    [privateKey]
  )
  let publicKey = useComputedState(
    () => (privateKeyIsValid ? getPublicKey(privateKey) : null),
    [privateKeyIsValid]
  )
  let signature = useComputedState(async () => {
    try {
      evt.pubkey = publicKey
      return await signEvent(evt, privateKey)
    } catch (err) {
      return null
    }
  }, [value, privateKey])
  let isValidSignature = useComputedState(async () => {
    if (evt.id && evt.signature) {
      try {
        return await verifySignature(evt)
      } catch (err) {
        return false
      }
    } else {
      return null
    }
  }, [value])

  return (
    <>
      <Item
        label="serialized event"
        hint="according to nip-01 signature algorithm"
      >
        {serializeEvent(evt)}
      </Item>
      <Item label="event id" hint="sha256 hash of serialized event">
        {getEventHash(evt)}
      </Item>
      {evt.sig ? (
        <Item label="signature valid">{isValidSignature.toString()}</Item>
      ) : (
        <>
          <Item
            label="private key"
            hint="paste any private key here (32 bytes hex-encoded)"
          >
            <input
              value={privateKey}
              onChange={e => setPrivateKey(e.target.value.toLowerCase())}
            />{' '}
            {privateKeyIsValid ? 'valid' : 'invalid'}
          </Item>
          <Item label="public key">{publicKey}</Item>
          <Item label="signature">{privateKeyIsValid ? signature : ''}</Item>
        </>
      )}
    </>
  )
}

EventSigning.match = value => {
  try {
    let evt = JSON.parse(value)
    return evt.kind && evt.content && evt.tags
  } catch (err) {
    /**/
  }
  return false
}
