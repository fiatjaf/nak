package app.handlers

import scala.util.matching.Regex
import scala.scalajs.js
import slinky.core.{FunctionalComponent}
import slinky.web.html._
import slinky.core.facade.Hooks._
import slinky.core.facade.Fragment

import app.handlers.{Handler}
import app.components.{Item}

object KeyHandling extends Handler {
  val keymatcher = "^[a-f0-9]{64}$".r

  override def handles(value: String): Boolean =
    keymatcher.matches(value.toLowerCase())

  override val component = FunctionalComponent[String] { props =>
    Fragment(
      Item.component(Item.props("private key", "", props)),
      Item.component(Item.props("public key", "", "soon to be shown here"))
    )
  }
}
