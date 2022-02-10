import React from 'react'

export default function Item({label, hint, children}) {
  return (
    <div
      style={{
        marginBottom: '9px',
        whiteSpace: 'pre-wrap',
        wordWrap: 'break-word',
        wordBreak: 'break-all'
      }}
    >
      <b data-wenk={hint} data-wenk-pos="right">
        {label}:{' '}
      </b>
      {children}
    </div>
  )
}
