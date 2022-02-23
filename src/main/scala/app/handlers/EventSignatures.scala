package app.handlers

import scala.scalajs.js
import slinky.core.FunctionalComponent
import slinky.web.html._
import slinky.core.facade.Hooks._
import slinky.core.facade.Fragment
import io.circe._
import io.circe.parser._

import app.handlers.{Handler}

object EventSignatures extends Handler {
  override def handles(value: String): Boolean = false

  override val component = FunctionalComponent[String] { props =>
    Fragment(
      "nada"
    )
  }
}
