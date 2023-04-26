import scala.util.Try
import io.circe.parser.*
import cats.syntax.all.*
import scodec.bits.ByteVector
import scoin.*
import snow.*

type Result = Either[
  String,
  Event | PrivateKey | AddressPointer | EventPointer | ProfilePointer |
    ByteVector32
]

object Parser {
  val additions = raw" *\+ *".r

  def parseInput(input: String): Result =
    if input == "" then Left("")
    else {
      val spl = additions.split(input)
      val result = ByteVector
        .fromHex(spl.head)
        .flatMap(b => Try(Right(ByteVector32(b))).toOption)
        .getOrElse(
          NIP19.decode(spl.head) match {
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
        )

      val extraRelays = spl
        .drop(1)
        .toList
        .filter(e => e.startsWith("wss://") || e.startsWith("ws://"))

      result.map {
        case a: AddressPointer => a.copy(relays = a.relays ::: extraRelays)
        case p: ProfilePointer => p.copy(relays = p.relays ::: extraRelays)
        case e: EventPointer   => e.copy(relays = e.relays ::: extraRelays)
        case r                 => r
      }
    }
}
