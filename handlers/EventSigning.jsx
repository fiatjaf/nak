import React from 'react'
import {getEventHash, serializeEvent} from 'nostr-tools'

import Item from '../components/item'

export default function EventSigning({value}) {
  let evt = JSON.parse(value)

  return (
    <>
      <Item label="serialized event">{serializeEvent(evt)}</Item>
      <Item label="event id" hint="sha256 hash of serialized">
        {getEventHash(evt)}
      </Item>
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
