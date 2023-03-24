import cats.data.*
import cats.effect.*
import cats.effect.syntax.all.*
import cats.syntax.all.*
import fs2.concurrent.*
import fs2.dom.{Event => _, *}
import io.circe.parser.*
import io.circe.syntax.*
import calico.*
import calico.html.io.{*, given}
import calico.syntax.*
import snow.*
import scoin.*

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
        div(
          cls := "w-4/5",
          h1(cls := "px-1 py-2 text-center text-xl", "nostr army knife"),
          div(
            cls := "flex justify-center my-3",
            input(store),
            button(
              cls :=
                "shrink bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 mx-2 px-4 rounded ",
              "generate event",
              onClick --> (_.foreach(_ =>
                store.input.set(
                  Event(
                    kind = 1,
                    content = "hello world"
                  ).sign(PrivateKey(randomBytes32()))
                    .asJson
                    .printWith(Utils.jsonPrinter)
                )
              ))
            )
          ),
          result(store)
        )
      )
  }

  def input(store: Store): Resource[IO, HtmlDivElement[IO]] =
    div(
      cls := "w-full grow",
      div(
        cls := "w-full flex justify-center",
        textArea.withSelf { self =>
          (
            cls := "w-full max-h-96 p-3 rounded",
            styleAttr := "min-height: 400px",
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
      cls := "w-full flex my-5",
      store.input.map { input =>
        if input.trim() == "" then div("")
        else
          decode[Event](input) match {
            case Left(err: io.circe.ParsingFailure) =>
              div("not valid JSON")
            case Left(err: io.circe.DecodingFailure) =>
              err.pathToRootString match {
                case None       => div(s"decoding ${err.pathToRootString}")
                case Some(path) => div(s"field $path is missing or wrong")
              }
            case Right(event) =>
              div(
                cls := "text-md",
                styleAttr := "font-family: monospace",
                div(
                  span(cls := "font-bold", "serialized event "),
                  event.serialized
                ),
                div(
                  span(cls := "font-bold", "implied event id "),
                  event.hash.toHex
                ),
                div(
                  span(
                    cls := "font-bold",
                    "does the implied event id match the given event id? "
                  ),
                  event.id == event.hash.toHex match {
                    case true => "yes"; case false => "no"
                  }
                ),
                div(
                  span(cls := "font-bold", "is signature valid? "),
                  event.isValid match { case true => "yes"; case false => "no" }
                )
              )
          }

      }
    )
}
