package app.handlers

import scala.scalajs.js
import slinky.core.FunctionalComponent
import slinky.web.html._
import slinky.core.facade.Hooks._
import slinky.core.facade.Fragment

import app.handlers.{Handler}

object Nothing extends Handler {
  override def handles(value: String): Boolean = true

  override val component = FunctionalComponent[String] { props =>
    Fragment(
      p("you can paste here"),
      ul(
        li("an unsigned event to be hashed and signed"),
        li("a signed event to have its signature checked"),
        li("a nostr relay URL to be inspected"),
        li("a nostr event id we'll try to fetch"),
        li("a nip05 identifier to be checked"),
        li(
          span("contribute a new function: "),
          a(
            target := "_blank",
            href := "https://github.com/fiatjaf/nostr-army-knife",
            style := js.Dynamic.literal(color = "inherit")
          )("_______")
        )
      )
    )
  }
}
