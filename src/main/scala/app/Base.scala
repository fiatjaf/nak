package app

import scala.scalajs.js
import org.scalajs.dom
import slinky.core.FunctionalComponent
import slinky.web.html._
import slinky.core.facade.Hooks._

import app.handlers.{Handler, Nothing}

object Base {
  val handlers: List[Handler] = List(Nothing)

  val component = FunctionalComponent[Unit] { props =>
    val (typedValue, setTypedValue) = useState("")

    useEffect(
      () => {
        val saved = dom.window.localStorage.getItem("value")
        println(s"saved: ${saved}")
        setTypedValue(saved match { case _: String => saved; case _ => "" })
      },
      Seq()
    )

    useEffect(
      () => {
        dom.window.localStorage.setItem("value", typedValue)
      },
      Seq(typedValue)
    )

    val Handler = handlers
      .find(handler => handler.handles(typedValue))
      .getOrElse(Nothing)

    div(
      style := js.Dynamic.literal(
        fontFamily = "monospace"
      )
    )(
      div(
        h1("nostr army knife"),
        p("paste something nostric"),
        textarea(
          value := typedValue,
          onChange := { ev => setTypedValue(ev.target.value) },
          style := js.Dynamic.literal(
            padding = "7px",
            width = "100%",
            minHeight = "200px"
          )
        )
      ),
      hr(style := js.Dynamic.literal(margin = "18px 0")),
      div(
        style := js.Dynamic.literal(
          width = "90%",
          margin = "auto"
        )
      )(
        Handler.component(typedValue)
      )
    )
  }
}
