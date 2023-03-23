import cats.effect.*
import cats.effect.syntax.all.*
import cats.syntax.all.*
import fs2.concurrent.*
import fs2.dom.{Event => _, *}
import io.circe.parser.*
import calico.*
import calico.html.io.{*, given}
import calico.syntax.*
import snow.*

object Generator {
  def apply(): Resource[IO, HtmlDivElement[IO]] =
    SignallingRef[IO]
      .of("")
      .toResource
      .flatMap { input =>
        div(
          cls := "my-3",
          button(cls := buttonCls, "generate event"),
          button(cls := buttonCls, "generate keypair")
        )
      }

  val buttonCls =
    "bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 mx-2 px-4 rounded"
}
