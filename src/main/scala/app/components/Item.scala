package app.components

import scala.scalajs.js
import slinky.core.{FunctionalComponent, CustomAttribute}
import slinky.web.html._

object Item {
  val wenkValue = CustomAttribute[String]("data-wenk")
  val wenkPos = CustomAttribute[String]("data-wenk-pos")

  case class props(label: String, hint: String, content: String)

  val component = FunctionalComponent[props] { props =>
    div(
      style := js.Dynamic.literal(
        marginBottom = "9px",
        whiteSpace = "pre-wrap",
        wordWrap = "break-word",
        wordBreak = "break-all"
      )
    )(
      b(wenkValue := props.hint, wenkPos := "right")(
        span(props.label)
      ),
      span(" "),
      span(props.content)
    )
  }
}
