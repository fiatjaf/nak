import React from 'react'
import useBooleanState from 'use-boolean-state'

import {getPublicKey} from 'nostr-tools'

import Item from '../components/item'

export default function KeyHandling({value}) {
  let privateKey = value
  let publicKey = getPublicKey(privateKey)

  return (
    <>
      <Item label="private key">{privateKey}</Item>
      <Item label="public key">{publicKey}</Item>
    </>
  )
}

KeyHandling.match = value => {
  try {
    if (value.toLowerCase().match(/^[a-f0-9]{64}$/)) return true
  } catch (err) {
    /**/
  }
  return false
}
