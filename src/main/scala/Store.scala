import cats.data.*
import cats.effect.*
import cats.effect.syntax.all.*
import cats.syntax.all.*
import fs2.concurrent.*
import fs2.dom.{Event => _, *}
import scoin.PrivateKey

case class Store(
    input: SignallingRef[IO, String],
    result: SignallingRef[IO, Result]
)

object Store {
  def apply(window: Window[IO]): Resource[IO, Store] = {
    val key = "nak-input"

    for {
      input <- SignallingRef[IO].of("").toResource
      result <- SignallingRef[IO, Result](Left("")).toResource

      _ <- Resource.eval {
        OptionT(window.localStorage.getItem(key))
          .foreachF(input.set(_))
      }

      _ <- window.localStorage
        .events(window)
        .foreach {
          case Storage.Event.Updated(`key`, _, value, _) =>
            input.set(value)
          case _ => IO.unit
        }
        .compile
        .drain
        .background

      _ <- input.discrete
        .evalTap(input => IO.cede *> window.localStorage.setItem(key, input))
        .evalTap(input => result.set(Parser.parseInput(input)))
        .compile
        .drain
        .background
    } yield Store(input, result)
  }
}
