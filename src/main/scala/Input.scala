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

object Input {
  def apply(): Resource[IO, HtmlDivElement[IO]] =
    SignallingRef[IO]
      .of("")
      .toResource
      .flatMap { input =>
        div(
          textArea.withSelf { self =>
            (
              cls := "w-2/3 min-h-fit max-h-96 p-3",
              placeholder := "paste something nostric",
              onInput --> (_.foreach(_ => self.value.get.flatMap(input.set))),
              value <-- input
            )
          },
          div(
            input.map(inp =>
              decode[Event](inp) match {
                case Left(err)    => err.toString
                case Right(event) => event.toString
              }
            )
          )
        )
      }

}
