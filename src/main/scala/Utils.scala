import io.circe.Printer
import scodec.bits.ByteVector
import scoin.*

object Utils {
  val keyOne = PrivateKey(ByteVector32(ByteVector(0x01).padLeft(32)))

  val jsonPrinter = Printer(
    dropNullValues = false,
    indent = "  ",
    lbraceRight = "\n",
    rbraceLeft = "\n",
    lbracketRight = "\n",
    rbracketLeft = "\n",
    lrbracketsEmpty = "",
    arrayCommaRight = "\n",
    objectCommaRight = "\n",
    colonLeft = "",
    colonRight = " ",
    sortKeys = true
  )
}
