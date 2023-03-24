import cats.data.*
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

object Store {
  def apply(window: Window[IO]): Resource[IO, Store] = {
    val key = "nak-input"

    for {
      inputRef <- SignallingRef[IO].of("").toResource

      _ <- Resource.eval {
        OptionT(window.localStorage.getItem(key))
          .foreachF(inputRef.set(_))
      }

      _ <- window.localStorage
        .events(window)
        .foreach {
          case Storage.Event.Updated(`key`, _, value, _) =>
            inputRef.set(value)
          case _ => IO.unit
        }
        .compile
        .drain
        .background

      _ <- inputRef.discrete
        .foreach(input => IO.cede *> window.localStorage.setItem(key, input))
        .compile
        .drain
        .background
    } yield new Store(inputRef)
  }
}

case class Store(input: SignallingRef[IO, String])

object Main extends IOWebApp {
  def render: Resource[IO, HtmlDivElement[IO]] = Store(window).flatMap {
    store =>
      div(
        cls := "flex w-full h-full flex-col items-center justify-center",
        h1(cls := "px-1 py-2 text-lg", "nostr army knife"),
        div(
          cls := "flex justify-center my-3"
          // button(cls := buttonCls, "generate event", onClick --> (_.foreach(_ => ))),
        ),
        input(store),
        result(store)
      )
  }

  def input(store: Store): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "w-full",
      div(
        cls := "w-full flex justify-center",
        textArea.withSelf { self =>
          (
            cls := "w-2/3 max-h-96 p-3",
            styleAttr := "min-height: 200px",
            placeholder := "paste something nostric",
            onInput --> (_.foreach(_ =>
              self.value.get.flatMap(store.input.set)
            )),
            value <-- store.input
          )
        }
      )
    )

  def result(store: Store): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "w-full flex justify-center",
      store.input.map { input =>
        if input.trim() == "" then ""
        else
          decode[Event](input) match {
            case Right(event) => event.toString
            case Left(err: io.circe.DecodingFailure) =>
              err.pathToRootString match {
                case Some(path) => s"field $path is missing or wrong"
                case None       => s"decoding ${err.pathToRootString}"
              }
            case Left(err: io.circe.ParsingFailure) =>
              "not valid JSON"
          }
      }
    )

  val buttonCls =
    "bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 mx-2 px-4 rounded"
}
