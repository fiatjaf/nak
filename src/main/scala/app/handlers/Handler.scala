package app.handlers

import slinky.core.FunctionalComponent

trait Handler {
  def handles(value: String): Boolean
  val component: FunctionalComponent[String]
}
