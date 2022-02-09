import React, {useState} from 'react'
import {render} from 'react-dom'

import Nothing from './handlers/Nothing'
import EventSigning from './handlers/EventSigning'

const handlers = [EventSigning, Nothing]

function App() {
  let [value, setValue] = useState(null)

  let Result
  for (let i = 0; i < handlers.length; i++) {
    let alt = handlers[i]
    if (alt.match(value)) {
      Result = alt
      break
    }
  }

  return (
    <main style={{fontFamily: 'monospace'}}>
      <div
        style={{
          width: '90%',
          margin: 'auto'
        }}
      >
        <h1>nostr army knife</h1>
        paste something nostric
        <textarea
          onChange={e => setValue(e.target.value)}
          style={{
            padding: '7px',
            width: '100%',
            minHeight: '200px'
          }}
        />
      </div>
      <hr style={{margin: '18px 0'}} />
      <div
        style={{
          width: '90%',
          margin: 'auto'
        }}
      >
        <Result value={value} />
      </div>
    </main>
  )
}

render(<App />, document.getElementById('app'))
