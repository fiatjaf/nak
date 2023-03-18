import calico.*
import calico.html.io.{*, given}
import calico.syntax.*
import calico.unsafe.given
import cats.effect.*
import cats.effect.syntax.all.*
import cats.syntax.all.*
import fs2.*
import fs2.concurrent.*
import fs2.dom.*

object Main extends IOWebApp {
  def Counter(
      label: String,
      initialStep: Int
  ): Resource[IO, HtmlDivElement[IO]] =
    SignallingRef[IO]
      .of(initialStep)
      .product(Channel.unbounded[IO, Int])
      .toResource
      .flatMap { (step, diff) =>
        val allowedSteps = List(1, 2, 3, 5, 10)

        div(
          p(
            "Step: ",
            select.withSelf { self =>
              (
                allowedSteps
                  .map(step => option(value := step.toString, step.toString)),
                value <-- step.map(_.toString),
                onChange --> {
                  _.evalMap(_ => self.value.get)
                    .map(_.toIntOption)
                    .unNone
                    .foreach(step.set)
                }
              )
            }
          ),
          p(
            label + ": ",
            b(diff.stream.scanMonoid.map(_.toString).holdOptionResource),
            " ",
            button(
              "-",
              onClick --> {
                _.evalMap(_ => step.get).map(-1 * _).foreach(diff.send(_).void)
              }
            ),
            button(
              "+",
              onClick --> (_.evalMap(_ => step.get).foreach(diff.send(_).void))
            )
          )
        )
      }

  def render: Resource[IO, HtmlDivElement[IO]] = div(
    h1("Let's count!"),
    Counter("Sheep", initialStep = 3)
  )
}
