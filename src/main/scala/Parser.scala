import io.circe.parser.*
import cats.syntax.all.*
import scoin.*
import snow.*

type Result = Either[
  String,
  Event | PrivateKey | AddressPointer | EventPointer | ProfilePointer
]

object Parser {
  def parseInput(input: String): Result =
    NIP19.decode(input) match {
      case Right(pp: ProfilePointer)   => Right(pp)
      case Right(evp: EventPointer)    => Right(evp)
      case Right(sk: PrivateKey)       => Right(sk)
      case Right(addr: AddressPointer) => Right(addr)
      case Left(_) =>
        parse(input) match {
          case Left(err: io.circe.ParsingFailure) =>
            Left("not valid JSON or NIP-19 code")
          case Right(json) =>
            json
              .as[Event]
              .leftMap { err =>
                err.pathToRootString match {
                  case None       => s"decoding ${err.pathToRootString}"
                  case Some(path) => s"field $path is missing or wrong"
                }
              }
        }
    }
}
